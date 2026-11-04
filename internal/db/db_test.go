package db_test

import (
	"math"
	"path/filepath"
	"testing"

	"wordhoard/internal/db"
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

func TestLexemeSourcesResolutionAndPreservation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open failed: %v", err)
	}
	defer store.Close()

	book, _, err := store.CreateBook(db.BookInput{
		Title:    "Source Layered Book",
		Author:   "Author S",
		Filename: "/books/sources.epub",
		FileSize: 4096,
		SHA256:   "sha-sources",
	})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}

	err = store.SaveBookLexemes(book.ID, []db.LexemeInput{
		{
			Lemma:            "laconic",
			POS:              "adjective",
			PromptForm:       "laconic (adj.)",
			CEFRLevel:        "c1",
			GlobalZipfFreq:   2.1,
			FullDefinition:   "1. Brief and to the point",
			ShortDefinitions: []string{"concise", "terse"},
			TranslationsLT:   []string{"lakoniškas"},
			OccurrenceCount:  3,
		},
	})
	if err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}

	// 1. After ingest (SaveBookLexemes), 'offline' source is populated and resolved
	page, err := store.ListBookVocabulary(book.ID, db.VocabularyFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListBookVocabulary: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	lexemeID := page.Items[0].ID

	sources, err := store.GetLexemeSources(lexemeID)
	if err != nil {
		t.Fatalf("GetLexemeSources: %v", err)
	}
	offlineSrc, ok := sources[db.SourceOffline]
	if !ok {
		t.Fatalf("expected 'offline' source row to exist after SaveBookLexemes")
	}
	if offlineSrc.FullDefinition != "1. Brief and to the point" ||
		len(offlineSrc.ShortDefinitions) != 2 || offlineSrc.ShortDefinitions[0] != "concise" ||
		len(offlineSrc.TranslationsLT) != 1 || offlineSrc.TranslationsLT[0] != "lakoniškas" {
		t.Errorf("unexpected offline source: %+v", offlineSrc)
	}

	// 2. Add 'online' source -> resolved lexeme combines offline + online (deduplicated union preserving order)
	err = store.UpsertLexemeSource(lexemeID, db.SourceOnline, db.SourceContent{
		FullDefinition:   "Online definition fallback",
		ShortDefinitions: []string{"terse", "pithy"},
		TranslationsLT:   []string{"lakoniškas", "glaustas", "trumpas"},
	})
	if err != nil {
		t.Fatalf("UpsertLexemeSource online: %v", err)
	}

	page, err = store.ListBookVocabulary(book.ID, db.VocabularyFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListBookVocabulary after online source: %v", err)
	}
	resolved := page.Items[0]
	if resolved.FullDefinition != "1. Brief and to the point" {
		t.Errorf("expected offline full_definition to take precedence when non-empty, got %q", resolved.FullDefinition)
	}
	wantShort := []string{"concise", "terse", "pithy"}
	if len(resolved.ShortDefinitions) != len(wantShort) {
		t.Fatalf("expected ShortDefinitions %v, got %v", wantShort, resolved.ShortDefinitions)
	}
	for i, w := range wantShort {
		if resolved.ShortDefinitions[i] != w {
			t.Errorf("ShortDefinitions[%d] = %q, want %q", i, resolved.ShortDefinitions[i], w)
		}
	}
	wantLT := []string{"lakoniškas", "glaustas", "trumpas"}
	if len(resolved.TranslationsLT) != len(wantLT) {
		t.Fatalf("expected TranslationsLT %v, got %v", wantLT, resolved.TranslationsLT)
	}
	for i, w := range wantLT {
		if resolved.TranslationsLT[i] != w {
			t.Errorf("TranslationsLT[%d] = %q, want %q", i, resolved.TranslationsLT[i], w)
		}
	}

	// Quiz candidates must also resolve combined offline + online values
	quiz, err := store.SelectQuizCandidates(book.ID, 10)
	if err != nil {
		t.Fatalf("SelectQuizCandidates: %v", err)
	}
	if len(quiz.Items) != 1 || len(quiz.Items[0].TranslationsLT) != 3 || quiz.Items[0].TranslationsLT[1] != "glaustas" {
		t.Errorf("expected quiz candidate to have resolved online translations, got %+v", quiz.Items)
	}

	// 3. Patching lexeme with user edits writes to source = 'user', overrides resolved values, and leaves 'offline' & 'online' untouched
	userDef := "1. Custom user definition"
	userShort := []string{"brief", "succinct"}
	userLT := []string{"glaustas"}
	updatedEntry, err := store.PatchLexeme(lexemeID, db.LexemePatch{
		FullDefinition:   &userDef,
		ShortDefinitions: &userShort,
		TranslationsLT:   &userLT,
	})
	if err != nil {
		t.Fatalf("PatchLexeme: %v", err)
	}
	if updatedEntry.FullDefinition != userDef ||
		len(updatedEntry.ShortDefinitions) != 2 || updatedEntry.ShortDefinitions[0] != "brief" ||
		len(updatedEntry.TranslationsLT) != 1 || updatedEntry.TranslationsLT[0] != "glaustas" {
		t.Errorf("unexpected updatedEntry from PatchLexeme: %+v", updatedEntry)
	}

	// Verify ListBookVocabulary and SelectQuizCandidates now return 'user' values
	page, err = store.ListBookVocabulary(book.ID, db.VocabularyFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListBookVocabulary after user edit: %v", err)
	}
	if page.Items[0].FullDefinition != userDef || len(page.Items[0].TranslationsLT) != 1 || page.Items[0].TranslationsLT[0] != "glaustas" {
		t.Errorf("expected ListBookVocabulary to return user source values, got %+v", page.Items[0])
	}

	quiz, err = store.SelectQuizCandidates(book.ID, 10)
	if err != nil {
		t.Fatalf("SelectQuizCandidates after user edit: %v", err)
	}
	if quiz.Items[0].FullDefinition != userDef || len(quiz.Items[0].ShortDefinitions) != 2 || quiz.Items[0].ShortDefinitions[0] != "brief" {
		t.Errorf("expected SelectQuizCandidates to return user source values, got %+v", quiz.Items[0])
	}

	// Verify underlying 'offline' and 'online' rows in lexeme_sources are completely untouched
	sourcesAfter, err := store.GetLexemeSources(lexemeID)
	if err != nil {
		t.Fatalf("GetLexemeSources after user edit: %v", err)
	}
	if len(sourcesAfter) != 3 {
		t.Fatalf("expected 3 sources (offline, online, user), got %d", len(sourcesAfter))
	}
	if sourcesAfter[db.SourceOffline].FullDefinition != "1. Brief and to the point" ||
		len(sourcesAfter[db.SourceOffline].TranslationsLT) != 1 || sourcesAfter[db.SourceOffline].TranslationsLT[0] != "lakoniškas" {
		t.Errorf("offline source was modified: %+v", sourcesAfter[db.SourceOffline])
	}
	if sourcesAfter[db.SourceOnline].FullDefinition != "Online definition fallback" ||
		len(sourcesAfter[db.SourceOnline].TranslationsLT) != 3 || sourcesAfter[db.SourceOnline].TranslationsLT[1] != "glaustas" {
		t.Errorf("online source was modified: %+v", sourcesAfter[db.SourceOnline])
	}
	if sourcesAfter[db.SourceUser].FullDefinition != userDef ||
		len(sourcesAfter[db.SourceUser].ShortDefinitions) != 2 || sourcesAfter[db.SourceUser].ShortDefinitions[0] != "brief" {
		t.Errorf("unexpected user source: %+v", sourcesAfter[db.SourceUser])
	}
}



