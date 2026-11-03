package db_test

import (
	"path/filepath"
	"testing"

	"ebook-vocab/internal/db"
)

func TestOpenConfiguresWALAndBusyTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer store.Close()

	var journalMode string
	if err := store.SQL().QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode = wal, got %q", journalMode)
	}

	var busyTimeout int
	if err := store.SQL().QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("expected busy_timeout = 5000, got %d", busyTimeout)
	}
}

func TestBookPersistenceAndDeduplicationQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer store.Close()

	hasSize, err := store.HasBookWithSize(1024)
	if err != nil {
		t.Fatalf("HasBookWithSize: %v", err)
	}
	if hasSize {
		t.Fatalf("expected HasBookWithSize(1024) to be false on empty db")
	}

	book, created, err := store.CreateBook(db.BookInput{
		Title:    "A Perfect Spy",
		Author:   "John le Carré",
		Filename: "/books/spy.epub",
		FileSize: 1024,
		SHA256:   "abc123sha",
	})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true on first insert")
	}
	if book.ID == 0 || book.Title != "A Perfect Spy" || book.Author != "John le Carré" {
		t.Errorf("unexpected book: %+v", book)
	}

	hasSize, err = store.HasBookWithSize(1024)
	if err != nil {
		t.Fatalf("HasBookWithSize: %v", err)
	}
	if !hasSize {
		t.Errorf("expected HasBookWithSize(1024) to be true after insert")
	}

	found, err := store.FindBookBySizeAndHash(1024, "abc123sha")
	if err != nil {
		t.Fatalf("FindBookBySizeAndHash: %v", err)
	}
	if found == nil || found.ID != book.ID {
		t.Fatalf("expected to find book %d, got %+v", book.ID, found)
	}

	if err := store.UpdateBookFilename(book.ID, "/books/renamed-spy.epub"); err != nil {
		t.Fatalf("UpdateBookFilename: %v", err)
	}

	books, err := store.ListBooks()
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].Filename != "/books/renamed-spy.epub" {
		t.Errorf("expected updated filename /books/renamed-spy.epub, got %q", books[0].Filename)
	}
	if books[0].TotalLexemes != 0 || books[0].KnownCount != 0 || books[0].MasteredCount != 0 {
		t.Errorf("expected 0 lexeme counts initially, got %+v", books[0])
	}
}

