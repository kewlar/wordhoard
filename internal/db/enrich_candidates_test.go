package db_test

import (
	"path/filepath"
	"testing"

	"wordhoard/internal/db"
)

func openEnrichStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func createEnrichBook(t *testing.T, store *db.Store, sha string, items []db.LexemeInput) int64 {
	t.Helper()
	book, _, err := store.CreateBook(db.BookInput{Title: sha, Author: "A", Filename: sha + ".epub", FileSize: 100, SHA256: sha})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := store.SaveBookLexemes(book.ID, items); err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}
	return book.ID
}

func lexemeIDByLemma(t *testing.T, store *db.Store, lemma string) int64 {
	t.Helper()
	var id int64
	if err := store.SQL().QueryRow(`SELECT id FROM lexemes WHERE lemma = ?`, lemma).Scan(&id); err != nil {
		t.Fatalf("lookup %s: %v", lemma, err)
	}
	return id
}

func candidateLemmas(entries []db.LexemeEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Lemma
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListEnrichCandidatesSkipsLexemesWithAnyResolvedTranslations(t *testing.T) {
	store := openEnrichStore(t)
	createEnrichBook(t, store, "book1", []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective", PromptForm: "laconic (adj.)", FullDefinition: "using few words"},
		{Lemma: "obdurate", POS: "adjective", PromptForm: "obdurate (adj.)", TranslationsLT: []string{"užsispyręs"}},
		{Lemma: "quixotic", POS: "adjective", PromptForm: "quixotic (adj.)"},
		{Lemma: "perfidy", POS: "noun", PromptForm: "a perfidy"},
	})

	if err := store.UpsertLexemeSource(lexemeIDByLemma(t, store, "quixotic"), db.SourceOnline, db.SourceContent{TranslationsLT: []string{"donkichotiškas"}}); err != nil {
		t.Fatalf("upsert online: %v", err)
	}
	if err := store.UpsertLexemeSource(lexemeIDByLemma(t, store, "perfidy"), db.SourceUser, db.SourceContent{TranslationsLT: []string{"klasta"}}); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	got, err := store.ListEnrichCandidates(db.EnrichFilter{})
	if err != nil {
		t.Fatalf("ListEnrichCandidates: %v", err)
	}
	if want := []string{"laconic"}; !equalStrings(candidateLemmas(got), want) {
		t.Fatalf("candidates = %v, want %v", candidateLemmas(got), want)
	}
	if got[0].FullDefinition != "using few words" || got[0].PromptForm != "laconic (adj.)" {
		t.Errorf("expected resolved lexeme details, got %+v", got[0])
	}
}

func TestListEnrichCandidatesFiltersByBookAndLimit(t *testing.T) {
	store := openEnrichStore(t)
	book1 := createEnrichBook(t, store, "book1", []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective", PromptForm: "laconic (adj.)"},
		{Lemma: "obdurate", POS: "adjective", PromptForm: "obdurate (adj.)"},
	})
	book2 := createEnrichBook(t, store, "book2", []db.LexemeInput{
		{Lemma: "perfidy", POS: "noun", PromptForm: "a perfidy"},
	})

	got, err := store.ListEnrichCandidates(db.EnrichFilter{BookID: book2})
	if err != nil {
		t.Fatalf("ListEnrichCandidates book2: %v", err)
	}
	if want := []string{"perfidy"}; !equalStrings(candidateLemmas(got), want) {
		t.Errorf("book2 candidates = %v, want %v", candidateLemmas(got), want)
	}

	got, err = store.ListEnrichCandidates(db.EnrichFilter{BookID: book1, Limit: 1})
	if err != nil {
		t.Fatalf("ListEnrichCandidates book1 limit: %v", err)
	}
	if want := []string{"laconic"}; !equalStrings(candidateLemmas(got), want) {
		t.Errorf("book1 limit 1 candidates = %v, want %v", candidateLemmas(got), want)
	}

	got, err = store.ListEnrichCandidates(db.EnrichFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListEnrichCandidates limit: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("limit 2 returned %d candidates", len(got))
	}
}
