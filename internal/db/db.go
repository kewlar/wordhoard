package db

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

type Book struct {
	ID            int64          `json:"id"`
	Title         string         `json:"title"`
	Author        string         `json:"author"`
	Filename      string         `json:"filename"`
	FileSize      int64          `json:"file_size"`
	SHA256        string         `json:"sha256"`
	ImportedAt    string         `json:"imported_at"`
	TotalLexemes  int            `json:"total_lexemes"`
	KnownCount    int            `json:"known_count"`
	MasteredCount int            `json:"mastered_count"`
	MasteryCounts map[string]int `json:"mastery_distribution"`
}

type BookInput struct {
	Title    string
	Author   string
	Filename string
	FileSize int64
	SHA256   string
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set journal_mode=WAL: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy_timeout=5000: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set foreign_keys=ON: %w", err)
	}
	s := &Store{db: sqlDB}
	if err := s.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SQL() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS books (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	author TEXT NOT NULL,
	filename TEXT NOT NULL,
	file_size INTEGER NOT NULL,
	sha256 TEXT NOT NULL,
	imported_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_books_size_sha256 ON books(file_size, sha256);

CREATE TABLE IF NOT EXISTS lexemes (
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

CREATE TABLE IF NOT EXISTS book_lexemes (
	book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	lexeme_id INTEGER NOT NULL REFERENCES lexemes(id) ON DELETE CASCADE,
	occurrence_count INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(book_id, lexeme_id)
);

CREATE TABLE IF NOT EXISTS excerpts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	lexeme_id INTEGER NOT NULL REFERENCES lexemes(id) ON DELETE CASCADE,
	chapter TEXT NOT NULL DEFAULT '',
	surface_word TEXT NOT NULL DEFAULT '',
	text TEXT NOT NULL DEFAULT '',
	highlight_start INTEGER NOT NULL DEFAULT 0,
	highlight_end INTEGER NOT NULL DEFAULT 0
);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) HasBookWithSize(fileSize int64) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM books WHERE file_size = ? LIMIT 1`, fileSize).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) FindBookBySizeAndHash(fileSize int64, sha256 string) (*Book, error) {
	var b Book
	err := s.db.QueryRow(
		`SELECT id, title, author, filename, file_size, sha256, imported_at
		 FROM books WHERE file_size = ? AND sha256 = ?`,
		fileSize, sha256,
	).Scan(&b.ID, &b.Title, &b.Author, &b.Filename, &b.FileSize, &b.SHA256, &b.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) UpdateBookFilename(bookID int64, filename string) error {
	_, err := s.db.Exec(`UPDATE books SET filename = ? WHERE id = ?`, filename, bookID)
	return err
}

func (s *Store) CreateBook(in BookInput) (*Book, bool, error) {
	existing, err := s.FindBookBySizeAndHash(in.FileSize, in.SHA256)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.Filename != in.Filename {
			if err := s.UpdateBookFilename(existing.ID, in.Filename); err != nil {
				return nil, false, err
			}
			existing.Filename = in.Filename
		}
		return existing, false, nil
	}

	res, err := s.db.Exec(
		`INSERT INTO books (title, author, filename, file_size, sha256) VALUES (?, ?, ?, ?, ?)`,
		in.Title, in.Author, in.Filename, in.FileSize, in.SHA256,
	)
	if err != nil {
		return nil, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, err
	}
	book, err := s.FindBookBySizeAndHash(in.FileSize, in.SHA256)
	if err != nil {
		return nil, false, err
	}
	if book == nil {
		book = &Book{
			ID:       id,
			Title:    in.Title,
			Author:   in.Author,
			Filename: in.Filename,
			FileSize: in.FileSize,
			SHA256:   in.SHA256,
		}
	}
	return book, true, nil
}

func (s *Store) ListBooks() ([]Book, error) {
	rows, err := s.db.Query(`
		SELECT
			b.id,
			b.title,
			b.author,
			b.filename,
			b.file_size,
			b.sha256,
			b.imported_at,
			COUNT(bl.lexeme_id) AS total_lexemes,
			COALESCE(SUM(CASE WHEN l.known = 1 THEN 1 ELSE 0 END), 0) AS known_count,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level >= 5 THEN 1 ELSE 0 END), 0) AS mastered_count,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level = 0 THEN 1 ELSE 0 END), 0) AS m0,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level = 1 THEN 1 ELSE 0 END), 0) AS m1,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level = 2 THEN 1 ELSE 0 END), 0) AS m2,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level = 3 THEN 1 ELSE 0 END), 0) AS m3,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level = 4 THEN 1 ELSE 0 END), 0) AS m4,
			COALESCE(SUM(CASE WHEN l.known = 0 AND l.mastery_level >= 5 THEN 1 ELSE 0 END), 0) AS m5
		FROM books b
		LEFT JOIN book_lexemes bl ON bl.book_id = b.id
		LEFT JOIN lexemes l ON l.id = bl.lexeme_id
		GROUP BY b.id
		ORDER BY b.imported_at DESC, b.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := make([]Book, 0)
	for rows.Next() {
		var b Book
		var m0, m1, m2, m3, m4, m5 int
		if err := rows.Scan(
			&b.ID, &b.Title, &b.Author, &b.Filename, &b.FileSize, &b.SHA256, &b.ImportedAt,
			&b.TotalLexemes, &b.KnownCount, &b.MasteredCount,
			&m0, &m1, &m2, &m3, &m4, &m5,
		); err != nil {
			return nil, err
		}
		b.MasteryCounts = map[string]int{
			"0": m0,
			"1": m1,
			"2": m2,
			"3": m3,
			"4": m4,
			"5": m5,
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return books, nil
}

