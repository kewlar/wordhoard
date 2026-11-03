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
