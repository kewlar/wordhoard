package server_test

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"ebook-vocab/internal/db"
	"ebook-vocab/internal/dictionary"
	"ebook-vocab/internal/ingest"
	"ebook-vocab/internal/server"
)

func createTestEPUB(t *testing.T, dir, filename, title, author string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
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
	writeEntry("OEBPS/ch1.xhtml", `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Hello world.</p></body>
</html>`)

	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return path
}

func TestGetBooksAndServeSPA(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	epubPath := createTestEPUB(t, tempDir, "spy.epub", "A Perfect Spy", "John le Carré")
	if _, err := ingest.Run(store, []string{epubPath}, nil); err != nil {
		t.Fatalf("ingest.Run: %v", err)
	}

	spaFS := fstest.MapFS{
		"index.html":        {Data: []byte("<!doctype html><html><body><div id=\"root\">Ebook Vocab SPA</div></body></html>")},
		"assets/app.js":     {Data: []byte("console.log('app');")},
	}

	handler := server.NewHandler(store, spaFS)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// 1. Verify GET /api/books
	resp, err := http.Get(ts.URL + "/api/books")
	if err != nil {
		t.Fatalf("GET /api/books: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Books []db.Book `json:"books"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/books response: %v", err)
	}
	if len(payload.Books) != 1 {
		t.Fatalf("expected 1 book in /api/books, got %d", len(payload.Books))
	}
	got := payload.Books[0]
	if got.Title != "A Perfect Spy" || got.Author != "John le Carré" {
		t.Errorf("unexpected book metadata: %+v", got)
	}
	if got.TotalLexemes != 0 || got.KnownCount != 0 || got.MasteredCount != 0 {
		t.Errorf("expected 0 lexeme counts initially, got %+v", got)
	}

	// 2. Verify GET / serves SPA index.html
	rootResp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer rootResp.Body.Close()
	if rootResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 on /, got %d", rootResp.StatusCode)
	}

	// 3. Verify client-side route fallback (e.g. /books/1/vocabulary) serves index.html
	routeResp, err := http.Get(ts.URL + "/books/1/vocabulary")
	if err != nil {
		t.Fatalf("GET /books/1/vocabulary: %v", err)
	}
	defer routeResp.Body.Close()
	if routeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 on SPA fallback route, got %d", routeResp.StatusCode)
	}
	if ct := routeResp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content-type on SPA fallback, got %q", ct)
	}
}

func createTestEPUBWithBody(t *testing.T, dir, filename, title, author, body string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
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

func TestGetBookVocabularyEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	fixtureDict := dictionary.NewFixture(
		[]dictionary.FixtureEntry{
			{
				Lemma:          "laconic",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "c1",
				GlobalZipfFreq: 2.0,
				Senses: []dictionary.Sense{
					{
						Definition:       "Using very few words",
						ShortDefinitions: []string{"concise", "terse"},
						TranslationsLT:   []string{"lakoniškas"},
					},
				},
			},
			{
				Lemma:          "ephemeral",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "",
				GlobalZipfFreq: 2.5,
				Senses: []dictionary.Sense{
					{
						Definition:       "Lasting for a very short time",
						ShortDefinitions: []string{"fleeting", "transient"},
						TranslationsLT:   []string{"efemeriškas"},
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
						Definition:       "A confusing and difficult problem",
						ShortDefinitions: []string{"riddle"},
						TranslationsLT:   []string{},
					},
				},
			},
		},
		nil,
	)

	book1 := createTestEPUBWithBody(t, tempDir, "b1.epub", "Book One", "Author One",
		"A laconic comment and a laconic answer met an ephemeral conundrum.")
	book2 := createTestEPUBWithBody(t, tempDir, "b2.epub", "Book Two", "Author Two",
		"An ephemeral moment passed.")

	if _, err := ingest.Run(store, []string{book1, book2}, nil, ingest.WithDictionary(fixtureDict)); err != nil {
		t.Fatalf("ingest.Run: %v", err)
	}

	handler := server.NewHandler(store, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// 1. Fetch full vocabulary for Book 1
	resp, err := http.Get(ts.URL + "/api/books/1/vocabulary")
	if err != nil {
		t.Fatalf("GET /api/books/1/vocabulary: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var page db.VocabularyPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if page.Book == nil || page.Book.Title != "Book One" || page.Book.TotalLexemes != 3 {
		t.Errorf("unexpected book metadata in vocabulary response: %+v", page.Book)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got total=%d len=%d", page.Total, len(page.Items))
	}

	// Order in Book 1:
	// 1. laconic (zipf=2.0, books=1/2, occ=2)
	// 2. conundrum (zipf=2.5, books=1/2, occ=1)
	// 3. ephemeral (zipf=2.5, books=2/2, occ=1)
	if page.Items[0].Lemma != "laconic" || page.Items[1].Lemma != "conundrum" || page.Items[2].Lemma != "ephemeral" {
		t.Errorf("expected [laconic, conundrum, ephemeral], got [%s, %s, %s]",
			page.Items[0].Lemma, page.Items[1].Lemma, page.Items[2].Lemma)
	}
	if page.Items[0].PromptForm != "laconic (adj.)" || len(page.Items[0].TranslationsLT) != 1 || page.Items[0].TranslationsLT[0] != "lakoniškas" {
		t.Errorf("unexpected first item fields: %+v", page.Items[0])
	}
	if len(page.Items[0].Excerpts) != 2 {
		t.Fatalf("expected 2 excerpts for 'laconic' in Book 1, got %d", len(page.Items[0].Excerpts))
	}
	for i, ex := range page.Items[0].Excerpts {
		if ex.Chapter != "Chapter 1" || ex.SurfaceWord != "laconic" {
			t.Errorf("excerpt %d: unexpected chapter/surface_word: %+v", i, ex)
		}
		if ex.Text[ex.HighlightStart:ex.HighlightEnd] != "laconic" {
			t.Errorf("excerpt %d: text[%d:%d] = %q, want 'laconic'", i, ex.HighlightStart, ex.HighlightEnd, ex.Text[ex.HighlightStart:ex.HighlightEnd])
		}
	}
	if page.Items[0].Excerpts[0].HighlightStart == page.Items[0].Excerpts[1].HighlightStart {
		t.Errorf("expected two distinct highlight positions for two occurrences of 'laconic', got %d", page.Items[0].Excerpts[0].HighlightStart)
	}

	// 2. Filter by pos=noun
	nounResp, err := http.Get(ts.URL + "/api/books/1/vocabulary?pos=noun")
	if err != nil {
		t.Fatalf("GET ?pos=noun: %v", err)
	}
	defer nounResp.Body.Close()
	var nounPage db.VocabularyPage
	if err := json.NewDecoder(nounResp.Body).Decode(&nounPage); err != nil {
		t.Fatalf("decode nounPage: %v", err)
	}
	if nounPage.Total != 1 || len(nounPage.Items) != 1 || nounPage.Items[0].Lemma != "conundrum" {
		t.Errorf("expected 1 noun conundrum, got %+v", nounPage)
	}

	// 3. Filter by search=ephem & pagination
	searchResp, err := http.Get(ts.URL + "/api/books/1/vocabulary?search=ephem&page=1&page_size=1")
	if err != nil {
		t.Fatalf("GET ?search=ephem: %v", err)
	}
	defer searchResp.Body.Close()
	var searchPage db.VocabularyPage
	if err := json.NewDecoder(searchResp.Body).Decode(&searchPage); err != nil {
		t.Fatalf("decode searchPage: %v", err)
	}
	if searchPage.Total != 1 || len(searchPage.Items) != 1 || searchPage.Items[0].Lemma != "ephemeral" {
		t.Errorf("expected 1 search result ephemeral, got %+v", searchPage)
	}

	// 4. Non-existent book returns 404
	notFoundResp, err := http.Get(ts.URL + "/api/books/999/vocabulary")
	if err != nil {
		t.Fatalf("GET /api/books/999/vocabulary: %v", err)
	}
	defer notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown book, got %d", notFoundResp.StatusCode)
	}
}

func TestKnownStatusMasteryTrackingAndQuizEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	fixtureDict := dictionary.NewFixture(
		[]dictionary.FixtureEntry{
			{
				Lemma:          "laconic",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "c1",
				GlobalZipfFreq: 2.0,
				Senses: []dictionary.Sense{
					{
						Definition:       "Using very few words",
						ShortDefinitions: []string{"concise", "terse"},
						TranslationsLT:   []string{"lakoniškas"},
					},
				},
			},
			{
				Lemma:          "ephemeral",
				POS:            dictionary.POSAdjective,
				CEFRLevel:      "",
				GlobalZipfFreq: 2.5,
				Senses: []dictionary.Sense{
					{
						Definition:       "Lasting for a very short time",
						ShortDefinitions: []string{"fleeting", "transient"},
						TranslationsLT:   []string{"efemeriškas"},
					},
				},
			},
			{
				Lemma:          "conundrum",
				POS:            dictionary.POSNoun,
				CEFRLevel:      "",
				GlobalZipfFreq: 2.2,
				Senses: []dictionary.Sense{
					{
						Definition:       "A confusing and difficult problem",
						ShortDefinitions: []string{"riddle"},
						TranslationsLT:   []string{"mįslė"},
					},
				},
			},
		},
		nil,
	)

	book1 := createTestEPUBWithBody(t, tempDir, "b1.epub", "Book One", "Author One",
		"A laconic comment and a laconic answer met an ephemeral conundrum.")
	book2 := createTestEPUBWithBody(t, tempDir, "b2.epub", "Book Two", "Author Two",
		"A laconic observer noticed an ephemeral moment.")

	if _, err := ingest.Run(store, []string{book1, book2}, nil, ingest.WithDictionary(fixtureDict)); err != nil {
		t.Fatalf("ingest.Run: %v", err)
	}

	handler := server.NewHandler(store, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Fetch Book 1 vocabulary to get lexeme IDs.
	// Order in Book 1 by obscurity_score DESC:
	// 1. laconic (zipf=2.0, books=2/2, occ=2 -> rarity=3.0, idf=ln(2)=0.693, sal=ln(3)=1.098 -> 4.79)
	// 2. conundrum (zipf=2.2, books=1/2, occ=1 -> rarity=2.8, idf=ln(3)=1.098, sal=ln(2)=0.693 -> 4.59)
	// 3. ephemeral (zipf=2.5, books=2/2, occ=1 -> rarity=2.5, idf=ln(2)=0.693, sal=ln(2)=0.693 -> 3.88)
	vocabResp, err := http.Get(ts.URL + "/api/books/1/vocabulary")
	if err != nil {
		t.Fatalf("GET /api/books/1/vocabulary: %v", err)
	}
	defer vocabResp.Body.Close()
	var vocabPage db.VocabularyPage
	if err := json.NewDecoder(vocabResp.Body).Decode(&vocabPage); err != nil {
		t.Fatalf("decode vocabPage: %v", err)
	}
	if len(vocabPage.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(vocabPage.Items))
	}
	laconicID := vocabPage.Items[0].ID
	conundrumID := vocabPage.Items[1].ID
	ephemeralID := vocabPage.Items[2].ID

	// 1. PATCH /api/lexemes/:id marks 'ephemeral' as known globally
	patchReq, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/api/lexemes/%d", ts.URL, ephemeralID), strings.NewReader(`{"known":true}`))
	if err != nil {
		t.Fatalf("new PATCH request: %v", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PATCH /api/lexemes/%d: %v", ephemeralID, err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from PATCH /api/lexemes/:id, got %d", patchResp.StatusCode)
	}

	// Verify GET /api/books/1/vocabulary hides known lexemes by default, and includes them when include_known=true
	hiddenResp, err := http.Get(ts.URL + "/api/books/1/vocabulary")
	if err != nil {
		t.Fatalf("GET /api/books/1/vocabulary: %v", err)
	}
	defer hiddenResp.Body.Close()
	var hiddenPage db.VocabularyPage
	if err := json.NewDecoder(hiddenResp.Body).Decode(&hiddenPage); err != nil {
		t.Fatalf("decode hiddenPage: %v", err)
	}
	if hiddenPage.Total != 2 || len(hiddenPage.Items) != 2 {
		t.Fatalf("expected 2 non-known items by default, got total=%d len=%d", hiddenPage.Total, len(hiddenPage.Items))
	}

	includedResp, err := http.Get(ts.URL + "/api/books/1/vocabulary?include_known=true")
	if err != nil {
		t.Fatalf("GET /api/books/1/vocabulary?include_known=true: %v", err)
	}
	defer includedResp.Body.Close()
	var includedPage db.VocabularyPage
	if err := json.NewDecoder(includedResp.Body).Decode(&includedPage); err != nil {
		t.Fatalf("decode includedPage: %v", err)
	}
	if includedPage.Total != 3 || len(includedPage.Items) != 3 {
		t.Fatalf("expected 3 items with include_known=true, got total=%d len=%d", includedPage.Total, len(includedPage.Items))
	}
	if !includedPage.Items[2].Known {
		t.Errorf("expected ephemeral to have Known=true, got %+v", includedPage.Items[2])
	}

	// Verify Book 2 also sees known_count = 1 globally via GET /api/books
	booksResp, err := http.Get(ts.URL + "/api/books")
	if err != nil {
		t.Fatalf("GET /api/books: %v", err)
	}
	defer booksResp.Body.Close()
	var booksPayload struct {
		Books []db.Book `json:"books"`
	}
	if err := json.NewDecoder(booksResp.Body).Decode(&booksPayload); err != nil {
		t.Fatalf("decode /api/books: %v", err)
	}
	for _, b := range booksPayload.Books {
		if b.KnownCount != 1 {
			t.Errorf("expected book %q to have known_count=1 globally, got %d", b.Title, b.KnownCount)
		}
	}

	// 2. POST /api/books/1/quiz excludes known lexemes ('ephemeral') and orders by mastery_level ASC, cooldown, obscurity_score DESC
	quizResp1, err := http.Post(ts.URL+"/api/books/1/quiz", "application/json", strings.NewReader(`{"size":20}`))
	if err != nil {
		t.Fatalf("POST /api/books/1/quiz: %v", err)
	}
	defer quizResp1.Body.Close()
	if quizResp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from POST /api/books/1/quiz, got %d", quizResp1.StatusCode)
	}
	var quizData1 struct {
		Book  *db.Book             `json:"book"`
		Items []db.VocabularyEntry `json:"items"`
	}
	if err := json.NewDecoder(quizResp1.Body).Decode(&quizData1); err != nil {
		t.Fatalf("decode quizData1: %v", err)
	}
	if len(quizData1.Items) != 2 {
		t.Fatalf("expected 2 quiz items (excluding known 'ephemeral'), got %d", len(quizData1.Items))
	}
	if quizData1.Items[0].ID != laconicID || quizData1.Items[1].ID != conundrumID {
		t.Errorf("expected quiz order [laconic, conundrum] by obscurity_score DESC, got [%s, %s]",
			quizData1.Items[0].Lemma, quizData1.Items[1].Lemma)
	}
	if len(quizData1.Items[0].Excerpts) == 0 {
		t.Errorf("expected quiz item to include excerpts, got 0")
	}

	// 3. POST /api/lexemes/:id/attempt with first_attempt_correct: false keeps mastery_level = 0 but updates last_quizzed_at
	// Because laconic was just quizzed (inside cooldown window), conundrum (never quizzed, mastery_level=0) should now come BEFORE laconic!
	attemptFalseResp, err := http.Post(
		fmt.Sprintf("%s/api/lexemes/%d/attempt", ts.URL, laconicID),
		"application/json",
		strings.NewReader(`{"first_attempt_correct":false}`),
	)
	if err != nil {
		t.Fatalf("POST attempt false: %v", err)
	}
	defer attemptFalseResp.Body.Close()
	if attemptFalseResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from POST attempt, got %d", attemptFalseResp.StatusCode)
	}

	quizResp2, err := http.Post(ts.URL+"/api/books/1/quiz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/books/1/quiz (after cooldown): %v", err)
	}
	defer quizResp2.Body.Close()
	var quizData2 struct {
		Items []db.VocabularyEntry `json:"items"`
	}
	if err := json.NewDecoder(quizResp2.Body).Decode(&quizData2); err != nil {
		t.Fatalf("decode quizData2: %v", err)
	}
	if len(quizData2.Items) != 2 || quizData2.Items[0].ID != conundrumID || quizData2.Items[1].ID != laconicID {
		t.Errorf("expected cooldown priority to order [conundrum, laconic], got [%s, %s]",
			quizData2.Items[0].Lemma, quizData2.Items[1].Lemma)
	}

	// 4. POST /api/lexemes/:id/attempt with first_attempt_correct: true increments mastery_level from 0 -> 1 -> 2,
	// and a wrong attempt resets mastery_level back to 0.
	for wantLevel := 1; wantLevel <= 2; wantLevel++ {
		resp, err := http.Post(
			fmt.Sprintf("%s/api/lexemes/%d/attempt", ts.URL, conundrumID),
			"application/json",
			strings.NewReader(`{"first_attempt_correct":true}`),
		)
		if err != nil {
			t.Fatalf("POST attempt true (%d): %v", wantLevel, err)
		}
		var res struct {
			ID           int64 `json:"id"`
			MasteryLevel int   `json:"mastery_level"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			resp.Body.Close()
			t.Fatalf("decode attempt response: %v", err)
		}
		resp.Body.Close()
		if res.MasteryLevel != wantLevel {
			t.Fatalf("expected mastery_level=%d, got %d", wantLevel, res.MasteryLevel)
		}
	}

	// Now laconic (mastery_level=0) has lower mastery_level than conundrum (mastery_level=2), so laconic is prioritized first!
	quizResp3, err := http.Post(ts.URL+"/api/books/1/quiz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/books/1/quiz (mastery ordering): %v", err)
	}
	defer quizResp3.Body.Close()
	var quizData3 struct {
		Items []db.VocabularyEntry `json:"items"`
	}
	if err := json.NewDecoder(quizResp3.Body).Decode(&quizData3); err != nil {
		t.Fatalf("decode quizData3: %v", err)
	}
	if quizData3.Items[0].ID != laconicID || quizData3.Items[1].ID != conundrumID {
		t.Errorf("expected mastery_level ASC to order [laconic(0), conundrum(2)], got [%s(%d), %s(%d)]",
			quizData3.Items[0].Lemma, quizData3.Items[0].MasteryLevel,
			quizData3.Items[1].Lemma, quizData3.Items[1].MasteryLevel)
	}

	// Reset conundrum to 0 on incorrect attempt
	resetResp, err := http.Post(
		fmt.Sprintf("%s/api/lexemes/%d/attempt", ts.URL, conundrumID),
		"application/json",
		strings.NewReader(`{"first_attempt_correct":false}`),
	)
	if err != nil {
		t.Fatalf("POST attempt reset: %v", err)
	}
	var resetData struct {
		MasteryLevel int `json:"mastery_level"`
	}
	_ = json.NewDecoder(resetResp.Body).Decode(&resetData)
	resetResp.Body.Close()
	if resetData.MasteryLevel != 0 {
		t.Fatalf("expected mastery_level reset to 0, got %d", resetData.MasteryLevel)
	}

	// 5. Reaching mastery_level = 5 (capped at 5) excludes the lexeme from future quizzes across all books
	for i := 0; i < 6; i++ {
		resp, err := http.Post(
			fmt.Sprintf("%s/api/lexemes/%d/attempt", ts.URL, laconicID),
			"application/json",
			strings.NewReader(`{"first_attempt_correct":true}`),
		)
		if err != nil {
			t.Fatalf("POST attempt true for laconic: %v", err)
		}
		var res struct {
			MasteryLevel int `json:"mastery_level"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()
		expected := i + 1
		if expected > 5 {
			expected = 5
		}
		if res.MasteryLevel != expected {
			t.Fatalf("attempt %d: expected mastery_level=%d, got %d", i+1, expected, res.MasteryLevel)
		}
	}

	// Now laconic is mastered (5) globally: Book 2 has only 'laconic' (mastered=5) and 'ephemeral' (known=true),
	// so POST /api/books/2/quiz returns 0 unmastered items, and Book 2 has mastered_count=1, known_count=1!
	book2QuizResp, err := http.Post(ts.URL+"/api/books/2/quiz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/books/2/quiz: %v", err)
	}
	defer book2QuizResp.Body.Close()
	var book2QuizData struct {
		Book  *db.Book             `json:"book"`
		Items []db.VocabularyEntry `json:"items"`
	}
	if err := json.NewDecoder(book2QuizResp.Body).Decode(&book2QuizData); err != nil {
		t.Fatalf("decode book2QuizData: %v", err)
	}
	if len(book2QuizData.Items) != 0 {
		t.Errorf("expected 0 unmastered items in Book 2 quiz, got %d", len(book2QuizData.Items))
	}
	if book2QuizData.Book == nil || book2QuizData.Book.MasteredCount != 1 || book2QuizData.Book.KnownCount != 1 {
		t.Errorf("expected Book 2 mastered_count=1 known_count=1, got %+v", book2QuizData.Book)
	}

	// 6. Verify 404 on unknown lexeme/book IDs for quiz, attempt, and patch
	unknownQuizResp, err := http.Post(ts.URL+"/api/books/999/quiz", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/books/999/quiz: %v", err)
	}
	unknownQuizResp.Body.Close()
	if unknownQuizResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown book quiz, got %d", unknownQuizResp.StatusCode)
	}

	unknownAttemptResp, err := http.Post(ts.URL+"/api/lexemes/999/attempt", "application/json", strings.NewReader(`{"first_attempt_correct":true}`))
	if err != nil {
		t.Fatalf("POST /api/lexemes/999/attempt: %v", err)
	}
	unknownAttemptResp.Body.Close()
	if unknownAttemptResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown lexeme attempt, got %d", unknownAttemptResp.StatusCode)
	}
}


