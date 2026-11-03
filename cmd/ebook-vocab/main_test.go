package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createSampleEPUB(t *testing.T, dir, filename, title, author string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create epub: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	writeEntry := func(name, content string) {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("create entry %s: %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("write entry %s: %v", name, err)
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
		t.Fatalf("close zip: %v", err)
	}
	return path
}

func TestCLIIngestSubcommand(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	epubPath := createSampleEPUB(t, tempDir, "call.epub", "Call for the Dead", "John le Carré")

	var out bytes.Buffer
	err := runCLI([]string{"ingest", "-db", dbPath, epubPath}, &out)
	if err != nil {
		t.Fatalf("runCLI ingest failed: %v", err)
	}
	if !strings.Contains(out.String(), "Call for the Dead") {
		t.Errorf("expected output to mention book title, got:\n%s", out.String())
	}
}
