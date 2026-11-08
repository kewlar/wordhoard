package db_test

import (
	"sort"
	"strconv"
	"testing"

	"wordhoard/internal/db"
)

type statusLex struct {
	lemma      string
	mastery    int
	quizzedAgo *int // days before now; nil = never quizzed
	known      bool
}

// statusFixture saves the given lexemes into a fresh book and applies their
// Mastery Level, quiz history and Known Status.
func statusFixture(t *testing.T, lexemes []statusLex) (*db.Store, int64) {
	t.Helper()
	store, bookID := scheduleFixture(t)
	inputs := make([]db.LexemeInput, 0, len(lexemes))
	for _, l := range lexemes {
		inputs = append(inputs, db.LexemeInput{
			Lemma: l.lemma, POS: "noun", GlobalZipfFreq: 1.0, OccurrenceCount: 1,
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
			t.Fatalf("set state for %s: %v", l.lemma, err)
		}
	}
	return store, bookID
}

func statusLexemes() []statusLex {
	days := func(n int) *int { return &n }
	return []statusLex{
		// Unseen: never quizzed.
		{lemma: "fresh", mastery: 0},
		// Learning, Not Due: failed today (level 0, 1-day interval → due tomorrow).
		{lemma: "failed", mastery: 0, quizzedAgo: days(0)},
		// Learning, Due: level 2 → 4-day interval, quizzed 5 days ago.
		{lemma: "overdue", mastery: 2, quizzedAgo: days(5)},
		// Learning, Due: level > 0 but never quizzed (legacy) → Due today.
		{lemma: "legacy", mastery: 3},
		// Learning, Not Due: level 4 → 16-day interval, quizzed today.
		{lemma: "pending", mastery: 4, quizzedAgo: days(0)},
		// Mastered and Due (level 5 → 32-day interval): not counted as due review.
		{lemma: "mastereddue", mastery: 5, quizzedAgo: days(40)},
		// Mastered, Not Due.
		{lemma: "masteredlater", mastery: 6, quizzedAgo: days(0)},
		// Known, regardless of history.
		{lemma: "knownquizzed", mastery: 2, quizzedAgo: days(10), known: true},
		{lemma: "knownfresh", mastery: 0, known: true},
	}
}

func TestBookLexemeCountsFollowLexemeStatus(t *testing.T) {
	store, bookID := statusFixture(t, statusLexemes())

	check := func(name string, b db.Book) {
		t.Helper()
		got := [6]int{b.TotalLexemes, b.KnownCount, b.MasteredCount, b.LearningCount, b.DueCount, b.UnseenCount}
		want := [6]int{9, 2, 2, 4, 2, 1}
		if got != want {
			t.Errorf("%s: [total known mastered learning due unseen] = %v, want %v", name, got, want)
		}
	}

	book, err := store.GetBook(bookID)
	if err != nil {
		t.Fatalf("GetBook: %v", err)
	}
	check("GetBook", *book)

	books, err := store.ListBooks()
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	check("ListBooks", books[0])
}

func TestVocabularyStatusFilterFollowsLexemeStatus(t *testing.T) {
	store, bookID := statusFixture(t, statusLexemes())

	cases := map[string][]string{
		"unseen":   {"fresh"},
		"learning": {"failed", "legacy", "overdue", "pending"},
		"mastered": {"mastereddue", "masteredlater"},
		"known":    {"knownfresh", "knownquizzed"},
	}
	for status, want := range cases {
		page, err := store.ListBookVocabulary(bookID, db.VocabularyFilter{Statuses: []string{status}})
		if err != nil {
			t.Fatalf("ListBookVocabulary(%s): %v", status, err)
		}
		got := make([]string, 0, len(page.Items))
		for _, it := range page.Items {
			got = append(got, it.Lemma)
		}
		sort.Strings(got)
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", status, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", status, got, want)
				break
			}
		}
	}
}
