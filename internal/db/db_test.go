package db_test

import (
	"math"
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

func TestDynamicObscurityScoreAndVocabularyFiltering(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer store.Close()

	book1, _, err := store.CreateBook(db.BookInput{
		Title:    "Book One",
		Author:   "Author A",
		Filename: "/books/one.epub",
		FileSize: 1001,
		SHA256:   "sha-one",
	})
	if err != nil {
		t.Fatalf("CreateBook 1: %v", err)
	}

	err = store.SaveBookLexemes(book1.ID, []db.LexemeInput{
		{
			Lemma:            "ataraxia",
			POS:              "noun",
			PromptForm:       "an ataraxia",
			CEFRLevel:        "",
			GlobalZipfFreq:   3.0,
			FullDefinition:   "1. Calmness untroubled by mental or emotional disquiet",
			ShortDefinitions: []string{"calmness", "serenity"},
			TranslationsLT:   []string{"ataraksija", "ramybė"},
			OccurrenceCount:  2,
		},
		{
			Lemma:            "zeitgeist",
			POS:              "noun",
			PromptForm:       "a zeitgeist",
			CEFRLevel:        "",
			GlobalZipfFreq:   3.0,
			FullDefinition:   "1. The spirit of the time",
			ShortDefinitions: []string{"spirit of the age"},
			TranslationsLT:   []string{},
			OccurrenceCount:  2,
		},
		{
			Lemma:            "laconic",
			POS:              "adjective",
			PromptForm:       "laconic (adj.)",
			CEFRLevel:        "c1",
			GlobalZipfFreq:   2.0,
			FullDefinition:   "1. Brief and to the point",
			ShortDefinitions: []string{"concise", "terse"},
			TranslationsLT:   []string{"lakoniškas"},
			OccurrenceCount:  4,
		},
	})
	if err != nil {
		t.Fatalf("SaveBookLexemes book1: %v", err)
	}

	// Now import Book Two which also contains "zeitgeist"
	book2, _, err := store.CreateBook(db.BookInput{
		Title:    "Book Two",
		Author:   "Author B",
		Filename: "/books/two.epub",
		FileSize: 2002,
		SHA256:   "sha-two",
	})
	if err != nil {
		t.Fatalf("CreateBook 2: %v", err)
	}

	err = store.SaveBookLexemes(book2.ID, []db.LexemeInput{
		{
			Lemma:            "zeitgeist",
			POS:              "noun",
			PromptForm:       "a zeitgeist",
			CEFRLevel:        "",
			GlobalZipfFreq:   3.0,
			FullDefinition:   "1. The spirit of the time",
			ShortDefinitions: []string{"spirit of the age"},
			TranslationsLT:   []string{},
			OccurrenceCount:  1,
		},
	})
	if err != nil {
		t.Fatalf("SaveBookLexemes book2: %v", err)
	}

	// Query Book 1 vocabulary without re-ingesting Book 1
	page, err := store.ListBookVocabulary(book1.ID, db.VocabularyFilter{
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListBookVocabulary: %v", err)
	}
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("expected 3 items in book1 vocabulary, got total=%d len=%d", page.Total, len(page.Items))
	}

	// Expected order in Book 1:
	// 1. "laconic": rarity=(5-2)=3.0, idf=ln(1+2/1)=ln(3), salience=ln(1+4)=ln(5) -> ~5.708
	// 2. "ataraxia": rarity=(5-3)=2.0, idf=ln(1+2/1)=ln(3), salience=ln(1+2)=ln(3) -> ~4.197
	// 3. "zeitgeist": rarity=(5-3)=2.0, idf=ln(1+2/2)=ln(2), salience=ln(1+2)=ln(3) -> ~3.792
	if page.Items[0].Lemma != "laconic" || page.Items[1].Lemma != "ataraxia" || page.Items[2].Lemma != "zeitgeist" {
		t.Fatalf("expected order [laconic, ataraxia, zeitgeist], got [%s, %s, %s]",
			page.Items[0].Lemma, page.Items[1].Lemma, page.Items[2].Lemma)
	}

	ataraxia := page.Items[1]
	wantRarity := 2.0
	wantIDF := math.Log(1.0 + 2.0/1.0)
	wantSalience := math.Log(1.0 + 2.0)
	wantScore := wantRarity + wantIDF + wantSalience
	if math.Abs(ataraxia.GlobalRarity-wantRarity) > 1e-6 ||
		math.Abs(ataraxia.LibraryIDF-wantIDF) > 1e-6 ||
		math.Abs(ataraxia.BookSalience-wantSalience) > 1e-6 ||
		math.Abs(ataraxia.ObscurityScore-wantScore) > 1e-6 {
		t.Errorf("unexpected score breakdown for ataraxia: %+v", ataraxia)
	}
	if ataraxia.BooksWithLexeme != 1 || ataraxia.TotalBooks != 2 || ataraxia.OccurrenceCount != 2 {
		t.Errorf("unexpected counts on ataraxia: books_with_lexeme=%d total_books=%d occ=%d",
			ataraxia.BooksWithLexeme, ataraxia.TotalBooks, ataraxia.OccurrenceCount)
	}

	zeitgeist := page.Items[2]
	if zeitgeist.BooksWithLexeme != 2 || zeitgeist.TotalBooks != 2 {
		t.Errorf("expected zeitgeist books_with_lexeme=2, total_books=2, got %d/%d",
			zeitgeist.BooksWithLexeme, zeitgeist.TotalBooks)
	}

	// Test POS filter
	adjPage, err := store.ListBookVocabulary(book1.ID, db.VocabularyFilter{
		Page:     1,
		PageSize: 10,
		POS:      "adjective",
	})
	if err != nil {
		t.Fatalf("ListBookVocabulary pos=adjective: %v", err)
	}
	if adjPage.Total != 1 || len(adjPage.Items) != 1 || adjPage.Items[0].Lemma != "laconic" {
		t.Errorf("expected 1 adjective 'laconic', got %+v", adjPage)
	}

	// Test search filter
	searchPage, err := store.ListBookVocabulary(book1.ID, db.VocabularyFilter{
		Page:     1,
		PageSize: 10,
		Search:   "zeit",
	})
	if err != nil {
		t.Fatalf("ListBookVocabulary search=zeit: %v", err)
	}
	if searchPage.Total != 1 || len(searchPage.Items) != 1 || searchPage.Items[0].Lemma != "zeitgeist" {
		t.Errorf("expected 1 search result 'zeitgeist', got %+v", searchPage)
	}

	// Test pagination
	paged, err := store.ListBookVocabulary(book1.ID, db.VocabularyFilter{
		Page:     2,
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("ListBookVocabulary page=2, page_size=2: %v", err)
	}
	if paged.Total != 3 || paged.TotalPages != 2 || len(paged.Items) != 1 || paged.Items[0].Lemma != "zeitgeist" {
		t.Errorf("unexpected page 2 result: %+v", paged)
	}
}


