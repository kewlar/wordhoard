package enrich_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"wordhoard/internal/db"
	"wordhoard/internal/enrich"
	"wordhoard/internal/translate"
)

type fakeTranslator struct {
	batches [][]translate.Request
	answers map[string][]string
	failOn  int // 1-based batch number that returns an error; 0 = never
}

func (f *fakeTranslator) TranslateBatch(_ context.Context, items []translate.Request) (map[int64][]string, error) {
	f.batches = append(f.batches, items)
	if f.failOn == len(f.batches) {
		return nil, errors.New("provider unavailable")
	}
	out := make(map[int64][]string)
	for _, it := range items {
		if lt, ok := f.answers[it.Lemma]; ok {
			out[it.ID] = lt
		}
	}
	return out, nil
}

func seedStore(t *testing.T, items []db.LexemeInput) (*db.Store, int64) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	book, _, err := store.CreateBook(db.BookInput{Title: "T", Author: "A", Filename: "t.epub", FileSize: 1, SHA256: "x"})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := store.SaveBookLexemes(book.ID, items); err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}
	return store, book.ID
}

func lexemeID(t *testing.T, store *db.Store, lemma string) int64 {
	t.Helper()
	var id int64
	if err := store.SQL().QueryRow(`SELECT id FROM lexemes WHERE lemma = ?`, lemma).Scan(&id); err != nil {
		t.Fatalf("lookup %s: %v", lemma, err)
	}
	return id
}

func TestRunTranslatesCandidatesInBatchesAndPersistsOnlineSource(t *testing.T) {
	store, _ := seedStore(t, []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective", FullDefinition: "using few words"},
		{Lemma: "obdurate", POS: "adjective", FullDefinition: "stubborn"},
		{Lemma: "perfidy", POS: "noun", FullDefinition: "treachery"},
		{Lemma: "known", POS: "adjective", TranslationsLT: []string{"žinomas"}},
	})
	tr := &fakeTranslator{answers: map[string][]string{
		"laconic":  {"lakoniškas"},
		"obdurate": {"užsispyręs", "kietaširdis"},
		"perfidy":  {"klasta"},
	}}

	var progress []enrich.Progress
	summary, err := enrich.Run(context.Background(), store, tr, enrich.Options{BatchSize: 2}, func(p enrich.Progress) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(tr.batches) != 2 || len(tr.batches[0]) != 2 || len(tr.batches[1]) != 1 {
		t.Fatalf("expected batches of sizes [2 1], got %d batches: %+v", len(tr.batches), tr.batches)
	}
	if tr.batches[0][0].Lemma != "laconic" || tr.batches[0][0].FullDefinition != "using few words" {
		t.Errorf("unexpected request payload: %+v", tr.batches[0][0])
	}
	if summary.Total != 3 || summary.Translated != 3 || summary.Failed != 0 {
		t.Errorf("unexpected summary: %+v", summary)
	}

	srcs, err := store.GetLexemeSources(lexemeID(t, store, "obdurate"))
	if err != nil {
		t.Fatalf("GetLexemeSources: %v", err)
	}
	if got := strings.Join(srcs[db.SourceOnline].TranslationsLT, ","); got != "užsispyręs,kietaširdis" {
		t.Errorf("online translations = %q", got)
	}
	if srcs[db.SourceOffline].FullDefinition != "stubborn" {
		t.Errorf("offline source must be preserved, got %+v", srcs[db.SourceOffline])
	}
	lex, err := store.GetLexeme(lexemeID(t, store, "perfidy"))
	if err != nil {
		t.Fatalf("GetLexeme: %v", err)
	}
	if strings.Join(lex.TranslationsLT, ",") != "klasta" {
		t.Errorf("resolved translations = %v", lex.TranslationsLT)
	}

	if len(progress) == 0 {
		t.Fatalf("expected progress callbacks")
	}
	last := progress[len(progress)-1]
	if last.Done != 3 || last.Total != 3 {
		t.Errorf("final progress = %+v, want done 3/3", last)
	}
}

func TestRunPersistsEachBatchBeforeTheNextAndCountsFailedBatches(t *testing.T) {
	store, _ := seedStore(t, []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective"},
		{Lemma: "obdurate", POS: "adjective"},
	})
	tr := &fakeTranslator{failOn: 2, answers: map[string][]string{
		"laconic":  {"lakoniškas"},
		"obdurate": {"užsispyręs"},
	}}

	laconicID := lexemeID(t, store, "laconic")
	savedBeforeSecondBatch := false
	summary, err := enrich.Run(context.Background(), store, tr, enrich.Options{BatchSize: 1}, func(p enrich.Progress) {
		if p.Done == 1 {
			lex, err := store.GetLexeme(laconicID)
			savedBeforeSecondBatch = err == nil && len(lex.TranslationsLT) == 1
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !savedBeforeSecondBatch {
		t.Errorf("expected first batch to be persisted before the second batch ran")
	}
	if summary.Translated != 1 || summary.Failed != 1 || len(summary.Errors) != 1 {
		t.Errorf("unexpected summary: %+v", summary)
	}

	remaining, err := store.ListEnrichCandidates(db.EnrichFilter{})
	if err != nil {
		t.Fatalf("ListEnrichCandidates: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Lemma != "obdurate" {
		t.Errorf("expected only the failed lexeme to remain a candidate, got %+v", remaining)
	}
}

func TestRunRespectsBookAndLimit(t *testing.T) {
	store, bookID := seedStore(t, []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective"},
		{Lemma: "obdurate", POS: "adjective"},
		{Lemma: "perfidy", POS: "noun"},
	})
	tr := &fakeTranslator{answers: map[string][]string{}}

	summary, err := enrich.Run(context.Background(), store, tr, enrich.Options{BookID: bookID, Limit: 2}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.Total != 2 || len(tr.batches) != 1 || len(tr.batches[0]) != 2 {
		t.Errorf("expected one batch of 2 with default batch size, got summary %+v batches %d", summary, len(tr.batches))
	}
	if summary.Empty != 2 {
		t.Errorf("expected 2 lexemes with no translations returned, got %+v", summary)
	}

	summary, err = enrich.Run(context.Background(), store, tr, enrich.Options{BookID: bookID + 99}, nil)
	if err != nil {
		t.Fatalf("Run other book: %v", err)
	}
	if summary.Total != 0 {
		t.Errorf("expected no candidates for an unknown book, got %+v", summary)
	}
}
