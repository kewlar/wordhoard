package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"wordhoard/internal/db"
	"wordhoard/internal/translate"
)

type cliFakeTranslator struct {
	batchSizes []int
}

func (f *cliFakeTranslator) TranslateBatch(_ context.Context, items []translate.TranslateRequest) (map[int64][]string, error) {
	f.batchSizes = append(f.batchSizes, len(items))
	out := make(map[int64][]string)
	for _, it := range items {
		out[it.ID] = []string{"lt-" + it.Lemma}
	}
	return out, nil
}

func useFakeTranslator(t *testing.T, tr translate.Translator, err error) {
	t.Helper()
	orig := newTranslator
	newTranslator = func() (translate.Translator, error) { return tr, err }
	t.Cleanup(func() { newTranslator = orig })
}

func seedCLIStore(t *testing.T, dbPath string) (book1, book2 int64) {
	t.Helper()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()
	mk := func(sha string, lemmas ...string) int64 {
		b, _, err := store.CreateBook(db.BookInput{Title: sha, Author: "A", Filename: sha, FileSize: 1, SHA256: sha})
		if err != nil {
			t.Fatalf("CreateBook: %v", err)
		}
		items := make([]db.LexemeInput, len(lemmas))
		for i, l := range lemmas {
			items[i] = db.LexemeInput{Lemma: l, POS: "noun", PromptForm: "a " + l}
		}
		if err := store.SaveBookLexemes(b.ID, items); err != nil {
			t.Fatalf("SaveBookLexemes: %v", err)
		}
		return b.ID
	}
	return mk("b1", "alpha", "beta", "gamma"), mk("b2", "delta")
}

func onlineTranslations(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()
	rows, err := store.SQL().Query(`
		SELECT l.lemma, ls.translations_lt_json FROM lexeme_sources ls
		JOIN lexemes l ON l.id = ls.lexeme_id WHERE ls.source = 'online'`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var lemma, js string
		if err := rows.Scan(&lemma, &js); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[lemma] = js
	}
	return out
}

func TestCLIEnrichTranslatesScopedLexemesInBatches(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	book1, _ := seedCLIStore(t, dbPath)
	tr := &cliFakeTranslator{}
	useFakeTranslator(t, tr, nil)

	var out bytes.Buffer
	err := runCLI([]string{"enrich", "-db", dbPath, "--book", itoa(book1), "--limit", "2", "--batch-size", "1", "--no-tui"}, &out)
	if err != nil {
		t.Fatalf("runCLI enrich: %v\n%s", err, out.String())
	}
	if len(tr.batchSizes) != 2 || tr.batchSizes[0] != 1 {
		t.Errorf("expected 2 batches of 1, got %v", tr.batchSizes)
	}
	got := onlineTranslations(t, dbPath)
	if len(got) != 2 || got["alpha"] != `["lt-alpha"]` || got["beta"] != `["lt-beta"]` {
		t.Errorf("unexpected online rows: %v", got)
	}
	for _, want := range []string{"2 / 2", "100%", "2 translated"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestCLIEnrichDefaultsToAllBooksAndReportsTranslatorConfigErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	seedCLIStore(t, dbPath)
	tr := &cliFakeTranslator{}
	useFakeTranslator(t, tr, nil)

	var out bytes.Buffer
	if err := runCLI([]string{"enrich", "-db", dbPath}, &out); err != nil {
		t.Fatalf("runCLI enrich: %v", err)
	}
	if len(tr.batchSizes) != 1 || tr.batchSizes[0] != 4 {
		t.Errorf("expected a single default-size batch of 4, got %v", tr.batchSizes)
	}

	useFakeTranslator(t, nil, errors.New("GEMINI_API_KEY is required"))
	err := runCLI([]string{"enrich", "-db", dbPath}, &out)
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Errorf("expected translator configuration error, got %v", err)
	}

	out.Reset()
	_ = runCLI([]string{"enrich", "-h"}, &out)
	for _, flagName := range []string{"-book", "-limit", "-batch-size", "-no-tui"} {
		if !strings.Contains(out.String(), flagName) {
			t.Errorf("help output missing %s:\n%s", flagName, out.String())
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
