package epub_test

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"wordhoard/internal/epub"
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
<body><p>A laconic answer followed the ephemeral sunset.</p></body>
</html>`)

	if err := w.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return path
}

func TestReadMetadata(t *testing.T) {
	dir := t.TempDir()
	epubPath := createTestEPUB(t, dir, "sample.epub", "The Laconic Chronicles", "Ada Lovelace")

	meta, err := epub.ReadMetadata(epubPath)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}
	if meta.Title != "The Laconic Chronicles" {
		t.Errorf("expected Title = %q, got %q", "The Laconic Chronicles", meta.Title)
	}
	if meta.Author != "Ada Lovelace" {
		t.Errorf("expected Author = %q, got %q", "Ada Lovelace", meta.Author)
	}
}

func TestReadRealEPUB(t *testing.T) {
	realPath := "/home/kewlar/Projects/ebook-vocabulary/ebooks/A Perfect Spy _ A Novel -- Carré, John Le (David John Moore Cornwell) -- ( WeLib.org ).epub"
	if _, err := os.Stat(realPath); os.IsNotExist(err) {
		t.Skip("sample epub not present on disk")
	}
	meta, err := epub.ReadMetadata(realPath)
	if err != nil {
		t.Fatalf("ReadMetadata on real epub failed: %v", err)
	}
	if meta.Title == "" || meta.Author == "" {
		t.Errorf("expected non-empty Title and Author, got %+v", meta)
	}
}

func TestReadChaptersExtractsTextFromSpine(t *testing.T) {
	dir := t.TempDir()
	epubPath := createTestEPUB(t, dir, "sample.epub", "The Laconic Chronicles", "Ada Lovelace")

	chapters, err := epub.ReadChapters(epubPath)
	if err != nil {
		t.Fatalf("ReadChapters failed: %v", err)
	}
	if len(chapters) != 1 {
		t.Fatalf("expected 1 chapter, got %d", len(chapters))
	}
	if chapters[0].Title != "Chapter 1" {
		t.Errorf("expected chapter title %q, got %q", "Chapter 1", chapters[0].Title)
	}
	if len(chapters[0].Paragraphs) != 1 || chapters[0].Paragraphs[0] != "A laconic answer followed the ephemeral sunset." {
		t.Errorf("unexpected paragraphs: %+v", chapters[0].Paragraphs)
	}
}


