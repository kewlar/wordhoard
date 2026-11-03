package ingest_test

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ebook-vocab/internal/db"
	"ebook-vocab/internal/ingest"
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

