package db_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"wordhoard/internal/db"
)

// scheduleFixture creates one book whose lexemes are each put into a given
// Mastery Level and last-quizzed offset (in days before now; nil = never).
func scheduleFixture(t *testing.T) (*db.Store, int64) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	book, _, err := store.CreateBook(db.BookInput{
		Title: "Schedule", Author: "A", Filename: "/books/s.epub", FileSize: 1, SHA256: "sha-s",
	})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	return store, book.ID
}

func TestSelectQuizCandidatesOrdersDueThenNewThenNotDue(t *testing.T) {
	store, bookID := scheduleFixture(t)

	type lex struct {
		lemma      string
		zipf       float64 // lower = more obscure
		mastery    int
		quizzedAgo *int // days before now; nil = never quizzed
		known      bool
	}
	days := func(n int) *int { return &n }
	lexemes := []lex{
		// Not Due: level 6 → 32-day interval, quizzed today.
		{lemma: "notduelater", zipf: 1.0, mastery: 6, quizzedAgo: days(0)},
		// Not Due: level 1 → 1-day interval, quizzed today → due tomorrow.
		{lemma: "notduesoon", zipf: 1.0, mastery: 1, quizzedAgo: days(0)},
		// New: never quizzed; ordered by obscurity.
		{lemma: "newcommon", zipf: 4.0, mastery: 0},
		{lemma: "newrare", zipf: 1.0, mastery: 0},
		// Due today, tie on Due Date → lowest Mastery Level first.
		{lemma: "legacy", zipf: 1.0, mastery: 3},                           // level > 0, never quizzed → Due now
		{lemma: "duelevelone", zipf: 1.0, mastery: 1, quizzedAgo: days(1)}, // 1-day interval
		{lemma: "failed", zipf: 4.0, mastery: 0, quizzedAgo: days(0)},      // 0-day interval
		// Overdue: level 2 → 2-day interval, quizzed 5 days ago → due 3 days ago.
		{lemma: "overdue", zipf: 1.0, mastery: 2, quizzedAgo: days(5)},
		// Most overdue, and above the old cap: level 5 → 16 days, quizzed 20 days ago.
		{lemma: "mastereddue", zipf: 4.0, mastery: 5, quizzedAgo: days(20)},
		// Known lexemes are never quizzed.
		{lemma: "knownword", zipf: 1.0, mastery: 0, known: true},
	}

	inputs := make([]db.LexemeInput, 0, len(lexemes))
	for _, l := range lexemes {
		inputs = append(inputs, db.LexemeInput{
			Lemma: l.lemma, POS: "noun", GlobalZipfFreq: l.zipf, OccurrenceCount: 1,
			FullDefinition: "1. x", ShortDefinitions: []string{"x"}, TranslationsLT: []string{"x"},
		})
	}
	if err := store.SaveBookLexemes(bookID, inputs); err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}
	for _, l := range lexemes {
		var quizzed any
		if l.quizzedAgo != nil {
			if err := store.SQL().QueryRow(
				`SELECT datetime('now', ?)`, "-"+strconv.Itoa(*l.quizzedAgo)+" days",
			).Scan(&quizzed); err != nil {
				t.Fatalf("compute timestamp: %v", err)
			}
		}
		if _, err := store.SQL().Exec(
			`UPDATE lexemes SET mastery_level = ?, last_quizzed_at = ?, known = ? WHERE lemma = ?`,
			l.mastery, quizzed, l.known, l.lemma,
		); err != nil {
			t.Fatalf("set schedule for %s: %v", l.lemma, err)
		}
	}

	session, err := store.SelectQuizCandidates(bookID, 50)
	if err != nil {
		t.Fatalf("SelectQuizCandidates: %v", err)
	}
	want := []string{
		"mastereddue", "overdue", "failed", "duelevelone", "legacy", // Due
		"newrare", "newcommon", // New
		"notduesoon", "notduelater", // Not Due
	}
	got := make([]string, 0, len(session.Items))
	for _, it := range session.Items {
		got = append(got, it.Lemma)
	}
	if len(got) != len(want) {
		t.Fatalf("quiz order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("quiz order = %v, want %v", got, want)
		}
	}
}

func TestRecordLexemeAttemptHasNoMasteryCap(t *testing.T) {
	store, bookID := scheduleFixture(t)
	if err := store.SaveBookLexemes(bookID, []db.LexemeInput{{
		Lemma: "laconic", POS: "adjective", GlobalZipfFreq: 2.0, OccurrenceCount: 1,
		FullDefinition: "1. x", ShortDefinitions: []string{"x"}, TranslationsLT: []string{"x"},
	}}); err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}
	var id int64
	if err := store.SQL().QueryRow(`SELECT id FROM lexemes WHERE lemma = 'laconic'`).Scan(&id); err != nil {
		t.Fatalf("lookup id: %v", err)
	}

	for want := 1; want <= 7; want++ {
		st, err := store.RecordLexemeAttempt(id, true, false)
		if err != nil {
			t.Fatalf("RecordLexemeAttempt: %v", err)
		}
		if st.MasteryLevel != want {
			t.Fatalf("after %d correct attempts: mastery = %d, want %d", want, st.MasteryLevel, want)
		}
	}

	// A wrong first attempt still resets to 0; a correction restores pre-attempt + 1, uncapped.
	if st, _ := store.RecordLexemeAttempt(id, false, false); st.MasteryLevel != 0 {
		t.Fatalf("after wrong attempt: mastery = %d, want 0", st.MasteryLevel)
	}
	if st, _ := store.RecordLexemeAttempt(id, true, true); st.MasteryLevel != 8 {
		t.Fatalf("after correction: mastery = %d, want 8", st.MasteryLevel)
	}
}
