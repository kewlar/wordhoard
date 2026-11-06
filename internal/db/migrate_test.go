package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"wordhoard/internal/db"
)

// Databases created before Prompt Form was retired carry a NOT NULL
// prompt_form column; opening them must drop it so lexeme inserts keep working.
func TestOpenDropsLegacyPromptFormColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	if _, err := legacy.Exec(`
CREATE TABLE lexemes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	lemma TEXT NOT NULL,
	pos TEXT NOT NULL,
	prompt_form TEXT NOT NULL,
	cefr_level TEXT,
	global_zipf_freq REAL NOT NULL DEFAULT 0,
	full_definition TEXT NOT NULL DEFAULT '',
	short_definitions_json TEXT NOT NULL DEFAULT '[]',
	translations_lt_json TEXT NOT NULL DEFAULT '[]',
	mastery_level INTEGER NOT NULL DEFAULT 0,
	known BOOLEAN NOT NULL DEFAULT 0,
	last_quizzed_at DATETIME,
	UNIQUE(lemma, pos)
);
INSERT INTO lexemes (lemma, pos, prompt_form) VALUES ('zeitgeist', 'noun', 'a zeitgeist');
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	legacy.Close()

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open on legacy db: %v", err)
	}
	defer store.Close()

	var count int
	if err := store.SQL().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('lexemes') WHERE name = 'prompt_form'`,
	).Scan(&count); err != nil {
		t.Fatalf("inspect lexemes columns: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected prompt_form column to be dropped")
	}

	book, _, err := store.CreateBook(db.BookInput{Title: "T", Author: "A", Filename: "/t.epub", FileSize: 1, SHA256: "s"})
	if err != nil {
		t.Fatalf("CreateBook: %v", err)
	}
	if err := store.SaveBookLexemes(book.ID, []db.LexemeInput{
		{Lemma: "zeitgeist", POS: "noun", OccurrenceCount: 1},
		{Lemma: "laconic", POS: "adjective", OccurrenceCount: 1},
	}); err != nil {
		t.Fatalf("SaveBookLexemes after migration: %v", err)
	}
}
