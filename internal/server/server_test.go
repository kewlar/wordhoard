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

