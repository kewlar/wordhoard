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
	if !strings.Contains(out.String(), "Wordhoard Ingester") {
		t.Errorf("expected output to mention Wordhoard Ingester, got:\n%s", out.String())
	}
}

func TestCLIWordhoardUsageAndDefaultDB(t *testing.T) {
	var out bytes.Buffer
	err := runCLI(nil, &out)
	if err == nil || !strings.Contains(err.Error(), "usage: wordhoard") {
		t.Fatalf("expected top-level usage error to mention wordhoard, got: %v", err)
	}

	err = runCLI([]string{"ingest"}, &out)
	if err == nil || !strings.Contains(err.Error(), "usage: wordhoard ingest") {
		t.Fatalf("expected ingest usage error to mention wordhoard ingest, got: %v", err)
	}

	out.Reset()
	_ = runCLI([]string{"serve", "-h"}, &out)
	if !strings.Contains(out.String(), "wordhoard.db") {
		t.Errorf("expected serve help output to default to wordhoard.db, got:\n%s", out.String())
	}

	tempDir := t.TempDir()
	epubPath := createSampleEPUB(t, tempDir, "call.epub", "Call for the Dead", "John le Carré")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	out.Reset()
	if err := runCLI([]string{"ingest", epubPath}, &out); err != nil {
		t.Fatalf("runCLI ingest with default db failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "wordhoard.db")); err != nil {
		t.Errorf("expected default database file wordhoard.db to be created, stat err: %v", err)
	}
}
