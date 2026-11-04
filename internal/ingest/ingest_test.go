package ingest_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wordhoard/internal/db"
	"wordhoard/internal/dictionary"
	"wordhoard/internal/ingest"
)

func createTestEPUB(t *testing.T, dir, filename, title, author, body string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test epub: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	writeEntry := func(name, content string) {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}

	writeEntry("mimetype", "application/epub+zip")
	writeEntry("META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)
	writeEntry("OEBPS/content.opf", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="bookid" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dc:language>en</dc:language>
    <dc:identifier id="bookid">urn:uuid:12345678-1234-1234-1234-1234567890ab</dc:identifier>
  </metadata>
  <manifest>
    <item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>
</package>`, title, author))
	writeEntry("OEBPS/ch1.xhtml", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>%s</p></body>
</html>`, body))

	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return path
}

func TestIngestGlobDeduplicationAndRenamedFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "vocab.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	booksDir := filepath.Join(tempDir, "books")
	book1Path := createTestEPUB(t, booksDir, "book1.epub", "A Perfect Spy", "John le Carré", "Magnus Pym vanished.")
	createTestEPUB(t, booksDir, "book2.epub", "Tinker Tailor Soldier Spy", "John le Carré", "George Smiley waited.")

	var stages []ingest.Status
	summary, err := ingest.Run(store, []string{filepath.Join(booksDir, "*.epub")}, func(ev ingest.Event) {
		stages = append(stages, ev.Status)
	})
	if err != nil {
		t.Fatalf("first ingest.Run failed: %v", err)
	}
	if summary.Imported != 2 || summary.Skipped != 0 {
		t.Fatalf("expected Imported=2, Skipped=0, got %+v", summary)
	}

	books, err := store.ListBooks()
	if err != nil {
		t.Fatalf("store.ListBooks: %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("expected 2 books in DB, got %d", len(books))
	}

	// Rename book1.epub to moved/renamed-spy.epub and re-ingest
	renamedPath := filepath.Join(tempDir, "moved", "renamed-spy.epub")
	if err := os.MkdirAll(filepath.Dir(renamedPath), 0o755); err != nil {
		t.Fatalf("mkdir moved: %v", err)
	}
	data, err := os.ReadFile(book1Path)
	if err != nil {
		t.Fatalf("read book1: %v", err)
	}
	if err := os.WriteFile(renamedPath, data, 0o644); err != nil {
		t.Fatalf("write renamed book1: %v", err)
	}

	summary2, err := ingest.Run(store, []string{renamedPath}, nil)
	if err != nil {
		t.Fatalf("second ingest.Run failed: %v", err)
	}
	if summary2.Imported != 0 || summary2.Skipped != 1 {
		t.Fatalf("expected Imported=0, Skipped=1 for renamed duplicate, got %+v", summary2)
	}

	booksAfter, err := store.ListBooks()
	if err != nil {
		t.Fatalf("store.ListBooks after rename: %v", err)
	}
	if len(booksAfter) != 2 {
		t.Fatalf("expected still 2 books in DB after re-ingesting renamed book, got %d", len(booksAfter))
	}

	var foundSpy *db.Book
	for i := range booksAfter {
		if booksAfter[i].Title == "A Perfect Spy" {
			foundSpy = &booksAfter[i]
			break
		}
	}
	if foundSpy == nil {
		t.Fatalf("could not find 'A Perfect Spy' in books list")
	}
	if foundSpy.Filename != renamedPath {
		t.Errorf("expected latest filename %q to be recorded, got %q", renamedPath, foundSpy.Filename)
	}
}

func TestRunTUIRendersProgressAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "vocab.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	epubPath := createTestEPUB(t, tempDir, "novel.epub", "Smiley's People", "John le Carré", "Ostrakova walked in Paris.")

	var out bytes.Buffer
	summary, err := ingest.RunTUI(store, []string{epubPath}, &out)
	if err != nil {
		t.Fatalf("RunTUI failed: %v", err)
	}
	if summary.Imported != 1 {
		t.Errorf("expected Imported=1, got %+v", summary)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "Done") || !strings.Contains(rendered, "novel.epub") {
		t.Errorf("expected TUI output to mention 'Done' and 'novel.epub', got:\n%s", rendered)
	}
}

func TestSameSizeDifferentContentBothImported(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "vocab.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	// Create two EPUBs with exact same byte length in their content so file_size matches,
	// while sha256 differs.
	p1 := createTestEPUB(t, tempDir, "alpha.epub", "Alpha Book", "Author One", "AAAA")
	p2 := createTestEPUB(t, tempDir, "bravo.epub", "Bravo Book", "Author Two", "BBBB")

	s1, _ := os.Stat(p1)
	s2, _ := os.Stat(p2)
	if s1.Size() != s2.Size() {
		// Pad whichever file is smaller at the tail of the zip so their file_size is identical
		if s1.Size() < s2.Size() {
			f, _ := os.OpenFile(p1, os.O_APPEND|os.O_WRONLY, 0o644)
			_, _ = f.Write(make([]byte, s2.Size()-s1.Size()))
			_ = f.Close()
		} else {
			f, _ := os.OpenFile(p2, os.O_APPEND|os.O_WRONLY, 0o644)
			_, _ = f.Write(make([]byte, s1.Size()-s2.Size()))
			_ = f.Close()
		}
	}

	summary, err := ingest.Run(store, []string{p1, p2}, nil)
	if err != nil {
		t.Fatalf("ingest.Run failed: %v", err)
	}
	if summary.Imported != 2 || summary.Skipped != 0 {
		t.Fatalf("expected both same-size distinct books to be imported, got %+v", summary)
	}
}

func TestIngestExtractsLearnableLexemesAndFiltersNoise(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	fixtureDict := dictionary.NewFixture(
		[]dictionary.FixtureEntry{
			{
				Lemma:          "walk",
				POS:            dictionary.POSVerb,
				CEFRLevel:      "a1",
				GlobalZipfFreq: 5.4,
				Senses: []dictionary.Sense{
					{Definition: "Move at a regular pace by lifting and setting down each foot", ShortDefinitions: []string{"stroll"}},
				},
			},
			{
				Lemma:          "house",
				POS:            dictionary.POSNoun,
				CEFRLevel:      "a1",
				GlobalZipfFreq: 5.8,
				Senses: []dictionary.Sense{
					{Definition: "A building for human habitation", ShortDefinitions: []string{"dwelling"}},
				},
			},
			{
				Lemma:          "laconic",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "c1",
				GlobalZipfFreq: 2.2,
				Senses: []dictionary.Sense{
					{
						Definition:       "Using very few words",
						ShortDefinitions: []string{"concise", "terse"},
						TranslationsLT:   []string{"lakoniškas", "glaustas"},
					},
				},
			},
			{
				Lemma:          "obfuscate",
				POS:            dictionary.POSVerb,
				CEFRLevel:      "",
				GlobalZipfFreq: 1.8,
				Senses: []dictionary.Sense{
					{
						Definition:       "Render obscure, unclear, or unintelligible",
						ShortDefinitions: []string{"obscure", "confuse"},
						TranslationsLT:   []string{},
					},
				},
			},
			{
				Lemma:          "conundrum",
				POS:            dictionary.POSNoun,
				CEFRLevel:      "",
				GlobalZipfFreq: 2.5,
				Senses: []dictionary.Sense{
					{
						Definition:       "A confusing and difficult problem or question",
						ShortDefinitions: []string{"riddle", "enigma"},
						TranslationsLT:   []string{"mįslė", "galvosūkis"},
					},
				},
			},
		},
		map[string]string{
			"walked":     "walk",
			"obfuscated": "obfuscate",
			"conundrums": "conundrum",
		},
	)

	text := "Magnus walked into the house in 1984. His laconic reply obfuscated the conundrums, and he obfuscated xyzzyspoon again."
	epubPath := createTestEPUB(t, tempDir, "spy.epub", "A Perfect Spy", "John le Carré", text)

	var out bytes.Buffer
	summary, err := ingest.RunTUI(store, []string{epubPath}, &out, ingest.WithDictionary(fixtureDict))
	if err != nil {
		t.Fatalf("ingest.RunTUI failed: %v", err)
	}
	if summary.Imported != 1 {
		t.Fatalf("expected Imported=1, got %+v", summary)
	}
	if summary.Files[0].LexemeCount != 3 {
		t.Fatalf("expected LexemeCount=3 (laconic, obfuscate, conundrum), got %d", summary.Files[0].LexemeCount)
	}
	if !strings.Contains(out.String(), "[3 lexemes]") {
		t.Errorf("expected TUI summary output to include '[3 lexemes]', got:\n%s", out.String())
	}

	books, err := store.ListBooks()
	if err != nil {
		t.Fatalf("store.ListBooks: %v", err)
	}
	if len(books) != 1 || books[0].TotalLexemes != 3 {
		t.Fatalf("expected 1 book with TotalLexemes=3, got %+v", books)
	}

	vocab, err := store.ListBookVocabulary(books[0].ID, db.VocabularyFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("store.ListBookVocabulary: %v", err)
	}
	if len(vocab.Items) != 3 {
		t.Fatalf("expected 3 lexeme entries, got %d", len(vocab.Items))
	}

	// obfuscate has lowest zipf (1.8 -> rarity 3.2) and highest occurrences (2 -> salience ln(3))
	if vocab.Items[0].Lemma != "obfuscate" || vocab.Items[0].PromptForm != "to obfuscate" || vocab.Items[0].OccurrenceCount != 2 {
		t.Errorf("expected top item to be 'to obfuscate' with count=2, got %+v", vocab.Items[0])
	}
	if vocab.Items[1].Lemma != "laconic" || vocab.Items[1].PromptForm != "laconic (adj.)" || vocab.Items[1].OccurrenceCount != 1 {
		t.Errorf("expected second item to be 'laconic (adj.)' with count=1, got %+v", vocab.Items[1])
	}
	if vocab.Items[2].Lemma != "conundrum" || vocab.Items[2].PromptForm != "a conundrum" || vocab.Items[2].OccurrenceCount != 1 {
		t.Errorf("expected third item to be 'a conundrum' with count=1, got %+v", vocab.Items[2])
	}
}

func TestIngestExtractsUpToFiveExcerptsWithAccurateOffsetsAndBoundedLongParagraphs(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	fixtureDict := dictionary.NewFixture(
		[]dictionary.FixtureEntry{
			{
				Lemma:          "obfuscate",
				POS:            dictionary.POSVerb,
				CEFRLevel:      "",
				GlobalZipfFreq: 1.8,
				Senses: []dictionary.Sense{
					{
						Definition:       "Render obscure, unclear, or unintelligible",
						ShortDefinitions: []string{"obscure", "confuse"},
					},
				},
			},
			{
				Lemma:          "laconic",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "c1",
				GlobalZipfFreq: 2.2,
				Senses: []dictionary.Sense{
					{
						Definition:       "Using very few words",
						ShortDefinitions: []string{"concise", "terse"},
					},
				},
			},
		},
		map[string]string{
			"obfuscated":  "obfuscate",
			"obfuscating": "obfuscate",
			"obfuscates":  "obfuscate",
		},
	)

	// Build an excessively long paragraph (> 600 chars) containing "laconic" in the middle
	longPrefix := strings.Repeat("The quiet diplomat walked along the misty riverbank in Vienna while pondering the archives. ", 5)
	longSuffix := strings.Repeat("Meanwhile the couriers waited near the station platform for the midnight express train. ", 5)
	longPara := longPrefix + "His “laconic” remark surprised everyone at the embassy." + " " + longSuffix

	body := strings.Join([]string{
		"First, he obfuscated the initial report.",
		"</p><p>Second, she obfuscates the ledger entries, and he obfuscated the cables too.",
		"</p><p>Fourth, they were obfuscating the trail in Berlin.",
		"</p><p>Fifth, nobody obfuscated the truth better.",
		"</p><p>Sixth, he obfuscated one more time beyond the cap.",
		"</p><p>" + longPara,
	}, "")

	epubPath := createTestEPUB(t, tempDir, "excerpts.epub", "Excerpt Test Book", "Author X", body)
	if _, err := ingest.Run(store, []string{epubPath}, nil, ingest.WithDictionary(fixtureDict)); err != nil {
		t.Fatalf("ingest.Run: %v", err)
	}

	vocab, err := store.ListBookVocabulary(1, db.VocabularyFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListBookVocabulary: %v", err)
	}
	if len(vocab.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(vocab.Items))
	}

	var obfuscateItem, laconicItem *db.LexemeEntry
	for i := range vocab.Items {
		switch vocab.Items[i].Lemma {
		case "obfuscate":
			obfuscateItem = &vocab.Items[i]
		case "laconic":
			laconicItem = &vocab.Items[i]
		}
	}
	if obfuscateItem == nil || laconicItem == nil {
		t.Fatalf("expected both obfuscate and laconic items")
	}

	// obfuscate appeared 6 times, so occurrence_count == 6, but excerpts must be capped at 5
	if obfuscateItem.OccurrenceCount != 6 {
		t.Errorf("expected obfuscate OccurrenceCount=6, got %d", obfuscateItem.OccurrenceCount)
	}
	if len(obfuscateItem.Excerpts) != 5 {
		t.Fatalf("expected 5 excerpts for obfuscate (5-excerpt cap), got %d", len(obfuscateItem.Excerpts))
	}

	wantSurface := []string{"obfuscated", "obfuscates", "obfuscated", "obfuscating", "obfuscated"}
	for i, ex := range obfuscateItem.Excerpts {
		if ex.Chapter != "Chapter 1" {
			t.Errorf("excerpt %d: expected Chapter='Chapter 1', got %q", i, ex.Chapter)
		}
		if ex.SurfaceWord != wantSurface[i] {
			t.Errorf("excerpt %d: expected SurfaceWord=%q, got %q", i, wantSurface[i], ex.SurfaceWord)
		}
		if ex.HighlightStart < 0 || ex.HighlightEnd > len(ex.Text) || ex.HighlightStart >= ex.HighlightEnd {
			t.Fatalf("excerpt %d: invalid offsets [%d:%d] for text len %d", i, ex.HighlightStart, ex.HighlightEnd, len(ex.Text))
		}
		gotSlice := ex.Text[ex.HighlightStart:ex.HighlightEnd]
		if gotSlice != ex.SurfaceWord {
			t.Errorf("excerpt %d: text[%d:%d] = %q, want %q (text=%q)", i, ex.HighlightStart, ex.HighlightEnd, gotSlice, ex.SurfaceWord, ex.Text)
		}
	}
	// Notice in the 2nd paragraph ("Second, she obfuscates the ledger entries, and he obfuscated the cables too."),
	// both occurrences are in the same paragraph; the 2nd excerpt highlights "obfuscates" and the 3rd highlights "obfuscated".
	if obfuscateItem.Excerpts[1].HighlightStart == obfuscateItem.Excerpts[2].HighlightStart {
		t.Errorf("expected distinct highlight offsets for two occurrences in the same paragraph, got %d and %d",
			obfuscateItem.Excerpts[1].HighlightStart, obfuscateItem.Excerpts[2].HighlightStart)
	}

	// Verify laconic excerpt from excessively long paragraph is bounded and accurately highlighted
	if len(laconicItem.Excerpts) != 1 {
		t.Fatalf("expected 1 excerpt for laconic, got %d", len(laconicItem.Excerpts))
	}
	lacEx := laconicItem.Excerpts[0]
	if len(lacEx.Text) >= len(longPara) || len(lacEx.Text) > 500 {
		t.Errorf("expected long paragraph (len %d) to be bounded (<= 500 chars), got len %d: %q", len(longPara), len(lacEx.Text), lacEx.Text)
	}
	if lacEx.Text[lacEx.HighlightStart:lacEx.HighlightEnd] != "laconic" {
		t.Errorf("expected bounded excerpt slice [%d:%d] to equal 'laconic', got %q in %q",
			lacEx.HighlightStart, lacEx.HighlightEnd, lacEx.Text[lacEx.HighlightStart:lacEx.HighlightEnd], lacEx.Text)
	}
}




func TestRunReportsFineGrainedStagesWithChapterProgress(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	epubPath := createTestEPUB(t, tempDir, "novel.epub", "The Honourable Schoolboy", "John le Carré", "Jerry Westerby wandered through the labyrinthine bazaar.")

	var events []ingest.Event
	if _, err := ingest.Run(store, []string{epubPath}, func(ev ingest.Event) {
		events = append(events, ev)
	}); err != nil {
		t.Fatalf("ingest.Run: %v", err)
	}

	var stages []ingest.Status
	for _, ev := range events {
		if len(stages) == 0 || stages[len(stages)-1] != ev.Status {
			stages = append(stages, ev.Status)
		}
	}
	want := []ingest.Status{ingest.StatusParsing, ingest.StatusReading, ingest.StatusAnalyzing, ingest.StatusSaving, ingest.StatusDone}
	if fmt.Sprint(stages) != fmt.Sprint(want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}

	var analyzing []ingest.Event
	for _, ev := range events {
		if ev.Status == ingest.StatusAnalyzing {
			analyzing = append(analyzing, ev)
		}
	}
	last := analyzing[len(analyzing)-1]
	if last.Total != 1 || last.Progress != 1 {
		t.Errorf("final Analyzing event progress = %d/%d, want 1/1", last.Progress, last.Total)
	}
	if analyzing[0].Progress != 0 || analyzing[0].Total != 1 {
		t.Errorf("first Analyzing event progress = %d/%d, want 0/1", analyzing[0].Progress, analyzing[0].Total)
	}
}
