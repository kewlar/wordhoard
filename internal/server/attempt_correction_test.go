package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"wordhoard/internal/db"
	"wordhoard/internal/server"
)

func postAttempt(t *testing.T, baseURL string, lexemeID int64, body string) db.LexemeProgress {
	t.Helper()
	resp, err := http.Post(fmt.Sprintf("%s/api/lexemes/%d/attempt", baseURL, lexemeID), "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST attempt %s: %v", body, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST attempt %s: status %d", body, resp.StatusCode)
	}
	var progress db.LexemeProgress
	if err := json.NewDecoder(resp.Body).Decode(&progress); err != nil {
		t.Fatalf("decode attempt response: %v", err)
	}
	return progress
}

func TestAttemptCorrectionRestoresPreAttemptMasteryPlusOne(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "vocab.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()
	book, _, err := store.CreateBook(db.BookInput{Title: "T", Author: "A", Filename: "t.epub", FileSize: 1, SHA256: "x"})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := store.SaveBookLexemes(book.ID, []db.LexemeInput{{Lemma: "laconic", POS: "adjective", PromptForm: "laconic (adj.)"}}); err != nil {
		t.Fatalf("SaveBookLexemes: %v", err)
	}
	var lexemeID int64
	if err := store.SQL().QueryRow(`SELECT id FROM lexemes WHERE lemma = 'laconic'`).Scan(&lexemeID); err != nil {
		t.Fatalf("lookup lexeme: %v", err)
	}

	ts := httptest.NewServer(server.NewHandler(store, nil))
	defer ts.Close()

	// Earlier sessions raise mastery to 3.
	for range 3 {
		postAttempt(t, ts.URL, lexemeID, `{"first_attempt_correct":true}`)
	}

	// Wrong first attempt in this session resets mastery to 0.
	if got := postAttempt(t, ts.URL, lexemeID, `{"first_attempt_correct":false}`).MasteryLevel; got != 0 {
		t.Fatalf("after wrong attempt: mastery = %d, want 0", got)
	}

	// Editing the lexeme makes the answer correct: the first attempt is
	// corrected, so mastery becomes the pre-attempt level (3) + 1.
	if got := postAttempt(t, ts.URL, lexemeID, `{"first_attempt_correct":true,"correction":true}`).MasteryLevel; got != 4 {
		t.Fatalf("after correction: mastery = %d, want 4", got)
	}
}
