package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wordhoard/internal/strlist"

	_ "modernc.org/sqlite"
)

var (
	ErrBookNotFound   = errors.New("book not found")
	ErrLexemeNotFound = errors.New("lexeme not found")
)


type Book struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Filename   string `json:"filename"`
	FileSize   int64  `json:"file_size"`
	SHA256     string `json:"sha256"`
	ImportedAt string `json:"imported_at"`
	// Lexeme counts by Lexeme Status; DueCount is the subset of
	// LearningCount whose Due Date is today or earlier.
	TotalLexemes  int `json:"total_lexemes"`
	KnownCount    int `json:"known_count"`
	MasteredCount int `json:"mastered_count"`
	LearningCount int `json:"learning_count"`
	DueCount      int `json:"due_count"`
	UnseenCount   int `json:"unseen_count"`
}

type BookInput struct {
	Title    string
	Author   string
	Filename string
	FileSize int64
	SHA256   string
}

type Excerpt struct {
	ID             int64  `json:"id"`
	Chapter        string `json:"chapter"`
	SurfaceWord    string `json:"surface_word"`
	Text           string `json:"text"`
	HighlightStart int    `json:"highlight_start"`
	HighlightEnd   int    `json:"highlight_end"`
}

type ExcerptInput struct {
	Chapter        string
	SurfaceWord    string
	Text           string
	HighlightStart int
	HighlightEnd   int
}

type LexemeInput struct {
	Lemma            string
	POS              string
	CEFRLevel        string
	GlobalZipfFreq   float64
	FullDefinition   string
	ShortDefinitions []string
	TranslationsLT   []string
	OccurrenceCount  int
	Excerpts         []ExcerptInput
}

type LexemeEntry struct {
	ID               int64     `json:"id"`
	Lemma            string    `json:"lemma"`
	POS              string    `json:"pos"`
	CEFRLevel        string    `json:"cefr_level"`
	ObscurityScore   float64   `json:"obscurity_score"`
	GlobalRarity     float64   `json:"global_rarity"`
	LibraryIDF       float64   `json:"library_idf"`
	BookSalience     float64   `json:"book_salience"`
	GlobalZipfFreq   float64   `json:"global_zipf_freq"`
	OccurrenceCount  int       `json:"occurrence_count"`
	BooksWithLexeme  int       `json:"books_with_lexeme"`
	TotalBooks       int       `json:"total_books"`
	FullDefinition   string    `json:"full_definition"`
	ShortDefinitions []string  `json:"short_definitions"`
	TranslationsLT   []string  `json:"translations_lt"`
	MasteryLevel     int       `json:"mastery_level"`
	Known            bool      `json:"known"`
	Excerpts         []Excerpt `json:"excerpts"`
}

type VocabularyFilter struct {
	Page         int
	PageSize     int
	POS          string
	Search       string
	IncludeKnown bool
	// Statuses filters by Lexeme Status keys (see lexemeStatusSQL). A nil
	// slice means no status filter (IncludeKnown applies instead); a non-nil
	// empty slice matches nothing.
	Statuses []string
}

// dueDateSQL is a lexeme's Due Date (GLOSSARY) as a local calendar day, over
// lexemes aliased as l: last quiz day + an interval of 0 days at Mastery
// Level 0 and 2^(level-1) days above it. NULL means New (never quizzed). A
// level > 0 with no quiz timestamp (legacy data) is Due today. The exponent is
// clamped only to keep the date within SQLite's range; real levels never get
// near it.
const dueDateSQL = `(CASE
	WHEN l.last_quizzed_at IS NULL AND l.mastery_level = 0 THEN NULL
	WHEN l.last_quizzed_at IS NULL THEN date('now', 'localtime')
	ELSE date(
		l.last_quizzed_at, 'localtime',
		'+' || (CASE WHEN l.mastery_level = 0 THEN 0
			ELSE (1 << min(l.mastery_level - 1, 20)) END) || ' days'
	)
END)`

// Lexeme Status (GLOSSARY) predicates over lexemes aliased as l. "Never
// quizzed" matches the New Review State: no quiz timestamp and Mastery Level 0.
const (
	unseenSQL   = "(l.known = 0 AND l.last_quizzed_at IS NULL AND l.mastery_level = 0)"
	learningSQL = "(l.known = 0 AND (l.last_quizzed_at IS NOT NULL OR l.mastery_level > 0) AND l.mastery_level < 5)"
	masteredSQL = "(l.known = 0 AND l.mastery_level >= 5)"
	knownSQL    = "(l.known = 1)"
)

var lexemeStatusSQL = map[string]string{
	"unseen":   unseenSQL,
	"learning": learningSQL,
	"mastered": masteredSQL,
	"known":    knownSQL,
}

type LexemeProgress struct {
	ID            int64  `json:"id"`
	Lemma         string `json:"lemma"`
	POS           string `json:"pos"`
	MasteryLevel  int    `json:"mastery_level"`
	Known         bool   `json:"known"`
	LastQuizzedAt string `json:"last_quizzed_at,omitempty"`
}

// SourceTier is the origin tier of a Lexeme Source (ADR-0001).
type SourceTier string

const (
	SourceOffline SourceTier = "offline"
	SourceOnline  SourceTier = "online"
	SourceUser    SourceTier = "user"
)

// SourceContent is the definitions and translations stored per Lexeme Source.
type SourceContent struct {
	FullDefinition   string   `json:"full_definition"`
	ShortDefinitions []string `json:"short_definitions"`
	TranslationsLT   []string `json:"translations_lt"`
}

type LexemeSource struct {
	LexemeID int64      `json:"lexeme_id"`
	Source   SourceTier `json:"source"`
	SourceContent
}

type LexemePatch struct {
	Known            *bool
	FullDefinition   *string
	ShortDefinitions *[]string
	TranslationsLT   *[]string
}

type QuizSession struct {
	Book  *Book         `json:"book"`
	Items []LexemeEntry `json:"items"`
}

type VocabularyPage struct {
	Book       *Book         `json:"book"`
	Items      []LexemeEntry `json:"items"`
	Total      int           `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
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
	cefr_level TEXT,
	global_zipf_freq REAL NOT NULL DEFAULT 0,
	full_definition TEXT NOT NULL DEFAULT '',
	short_definitions_json TEXT NOT NULL DEFAULT '[]',
	translations_lt_json TEXT NOT NULL DEFAULT '[]',
	mastery_level INTEGER NOT NULL DEFAULT 0,
	known BOOLEAN NOT NULL DEFAULT 0,
	last_quizzed_at DATETIME,
	pre_attempt_mastery_level INTEGER,
	UNIQUE(lemma, pos)
);

CREATE TABLE IF NOT EXISTS book_lexemes (
	book_id INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
	lexeme_id INTEGER NOT NULL REFERENCES lexemes(id) ON DELETE CASCADE,
	occurrence_count INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY(book_id, lexeme_id)
);

CREATE INDEX IF NOT EXISTS idx_book_lexemes_lexeme_id ON book_lexemes(lexeme_id);


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

CREATE INDEX IF NOT EXISTS idx_excerpts_book_lexeme ON excerpts(book_id, lexeme_id);

CREATE TABLE IF NOT EXISTS lexeme_sources (
	lexeme_id INTEGER NOT NULL REFERENCES lexemes(id) ON DELETE CASCADE,
	source TEXT NOT NULL CHECK(source IN ('offline', 'online', 'user')),
	full_definition TEXT NOT NULL DEFAULT '',
	short_definitions_json TEXT NOT NULL DEFAULT '[]',
	translations_lt_json TEXT NOT NULL DEFAULT '[]',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (lexeme_id, source)
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("lexemes", "pre_attempt_mastery_level", "INTEGER"); err != nil {
		return err
	}
	return s.dropColumnIfPresent("lexemes", "prompt_form")
}

// dropColumnIfPresent upgrades databases created while a since-removed column
// was still part of the CREATE TABLE schema.
func (s *Store) dropColumnIfPresent(table, column string) error {
	present, err := s.hasColumn(table, column)
	if err != nil || !present {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, column))
	return err
}

func (s *Store) hasColumn(table, column string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column,
	).Scan(&count)
	return count > 0, err
}

// addColumnIfMissing upgrades databases created before a column was added to
// the CREATE TABLE schema.
func (s *Store) addColumnIfMissing(table, column, decl string) error {
	present, err := s.hasColumn(table, column)
	if err != nil || present {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl))
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

// bookSelectSQL selects books with their lexeme counts by Lexeme Status, in
// scanBook's column order. Callers append WHERE, GROUP BY b.id and ORDER BY.
const bookSelectSQL = `
	SELECT
		b.id,
		b.title,
		b.author,
		b.filename,
		b.file_size,
		b.sha256,
		b.imported_at,
		COUNT(bl.lexeme_id),
		COALESCE(SUM(CASE WHEN ` + knownSQL + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + masteredSQL + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + learningSQL + ` THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + learningSQL + ` AND ` + dueDateSQL + ` <= date('now', 'localtime') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN ` + unseenSQL + ` THEN 1 ELSE 0 END), 0)
	FROM books b
	LEFT JOIN book_lexemes bl ON bl.book_id = b.id
	LEFT JOIN lexemes l ON l.id = bl.lexeme_id
`

func scanBook(row interface{ Scan(...any) error }) (Book, error) {
	var b Book
	err := row.Scan(
		&b.ID, &b.Title, &b.Author, &b.Filename, &b.FileSize, &b.SHA256, &b.ImportedAt,
		&b.TotalLexemes, &b.KnownCount, &b.MasteredCount, &b.LearningCount, &b.DueCount, &b.UnseenCount,
	)
	return b, err
}

func (s *Store) ListBooks() ([]Book, error) {
	rows, err := s.db.Query(bookSelectSQL + `
		GROUP BY b.id
		ORDER BY b.imported_at DESC, b.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	books := make([]Book, 0)
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return books, nil
}

func (s *Store) GetBook(bookID int64) (*Book, error) {
	b, err := scanBook(s.db.QueryRow(bookSelectSQL+`
		WHERE b.id = ?
		GROUP BY b.id
	`, bookID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) SaveBookLexemes(bookID int64, items []LexemeInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	upsertLexemeStmt, err := tx.Prepare(`
		INSERT INTO lexemes (
			lemma, pos, cefr_level, global_zipf_freq,
			full_definition, short_definitions_json, translations_lt_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(lemma, pos) DO UPDATE SET
			cefr_level = excluded.cefr_level,
			global_zipf_freq = excluded.global_zipf_freq,
			full_definition = excluded.full_definition,
			short_definitions_json = excluded.short_definitions_json,
			translations_lt_json = excluded.translations_lt_json
	`)
	if err != nil {
		return err
	}
	defer upsertLexemeStmt.Close()

	selectLexemeIDStmt, err := tx.Prepare(`SELECT id FROM lexemes WHERE lemma = ? AND pos = ?`)
	if err != nil {
		return err
	}
	defer selectLexemeIDStmt.Close()

	upsertBookLexemeStmt, err := tx.Prepare(`
		INSERT INTO book_lexemes (book_id, lexeme_id, occurrence_count)
		VALUES (?, ?, ?)
		ON CONFLICT(book_id, lexeme_id) DO UPDATE SET
			occurrence_count = excluded.occurrence_count
	`)
	if err != nil {
		return err
	}
	defer upsertBookLexemeStmt.Close()

	deleteExcerptsStmt, err := tx.Prepare(`DELETE FROM excerpts WHERE book_id = ? AND lexeme_id = ?`)
	if err != nil {
		return err
	}
	defer deleteExcerptsStmt.Close()

	insertExcerptStmt, err := tx.Prepare(`
		INSERT INTO excerpts (book_id, lexeme_id, chapter, surface_word, text, highlight_start, highlight_end)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer insertExcerptStmt.Close()

	upsertOfflineSourceStmt, err := tx.Prepare(upsertLexemeSourceSQL)
	if err != nil {
		return err
	}
	defer upsertOfflineSourceStmt.Close()

	for _, item := range items {
		shortJSON, err := encodeStringList(item.ShortDefinitions)
		if err != nil {
			return err
		}
		ltJSON, err := encodeStringList(item.TranslationsLT)
		if err != nil {
			return err
		}

		if _, err := upsertLexemeStmt.Exec(
			item.Lemma,
			item.POS,
			item.CEFRLevel,
			item.GlobalZipfFreq,
			item.FullDefinition,
			shortJSON,
			ltJSON,
		); err != nil {
			return err
		}

		var lexemeID int64
		if err := selectLexemeIDStmt.QueryRow(item.Lemma, item.POS).Scan(&lexemeID); err != nil {
			return err
		}

		if _, err := upsertOfflineSourceStmt.Exec(
			lexemeID,
			SourceOffline,
			item.FullDefinition,
			shortJSON,
			ltJSON,
		); err != nil {
			return err
		}

		if _, err := upsertBookLexemeStmt.Exec(bookID, lexemeID, item.OccurrenceCount); err != nil {
			return err
		}

		if _, err := deleteExcerptsStmt.Exec(bookID, lexemeID); err != nil {
			return err
		}

		excerpts := item.Excerpts
		if len(excerpts) > 5 {
			excerpts = excerpts[:5]
		}
		for _, ex := range excerpts {
			if _, err := insertExcerptStmt.Exec(
				bookID,
				lexemeID,
				ex.Chapter,
				ex.SurfaceWord,
				ex.Text,
				ex.HighlightStart,
				ex.HighlightEnd,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLexemeEntry(scanner rowScanner) (LexemeEntry, error) {
	var entry LexemeEntry
	var shortJSON, ltJSON string
	if err := scanner.Scan(
		&entry.ID,
		&entry.Lemma,
		&entry.POS,
		&entry.CEFRLevel,
		&entry.GlobalRarity,
		&entry.LibraryIDF,
		&entry.BookSalience,
		&entry.ObscurityScore,
		&entry.GlobalZipfFreq,
		&entry.OccurrenceCount,
		&entry.BooksWithLexeme,
		&entry.TotalBooks,
		&entry.FullDefinition,
		&shortJSON,
		&ltJSON,
		&entry.MasteryLevel,
		&entry.Known,
	); err != nil {
		return LexemeEntry{}, err
	}
	entry.ShortDefinitions = decodeStringList(shortJSON)
	entry.TranslationsLT = decodeStringList(ltJSON)
	entry.Excerpts = []Excerpt{}
	return entry, nil
}

func (s *Store) attachExcerpts(bookID int64, entries []LexemeEntry) error {
	if len(entries) == 0 {
		return nil
	}
	exRows, err := s.db.Query(`
		SELECT id, lexeme_id, chapter, surface_word, text, highlight_start, highlight_end
		FROM excerpts
		WHERE book_id = ?
		ORDER BY id ASC
	`, bookID)
	if err != nil {
		return err
	}
	defer exRows.Close()

	byLexeme := make(map[int64][]Excerpt)
	for exRows.Next() {
		var ex Excerpt
		var lexID int64
		if err := exRows.Scan(&ex.ID, &lexID, &ex.Chapter, &ex.SurfaceWord, &ex.Text, &ex.HighlightStart, &ex.HighlightEnd); err != nil {
			return err
		}
		byLexeme[lexID] = append(byLexeme[lexID], ex)
	}
	if err := exRows.Err(); err != nil {
		return err
	}

	for i := range entries {
		if exList, ok := byLexeme[entries[i].ID]; ok {
			entries[i].Excerpts = exList
		}
	}
	return nil
}

// decodeStringList decodes a JSON string array column, treating malformed or
// null values as empty.
func decodeStringList(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

// encodeStringList encodes a string list column, storing nil as `[]`.
func encodeStringList(list []string) (string, error) {
	if list == nil {
		list = []string{}
	}
	b, err := json.Marshal(list)
	return string(b), err
}

const upsertLexemeSourceSQL = `
		INSERT INTO lexeme_sources (
			lexeme_id, source, full_definition, short_definitions_json, translations_lt_json, updated_at
		) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(lexeme_id, source) DO UPDATE SET
			full_definition = excluded.full_definition,
			short_definitions_json = excluded.short_definitions_json,
			translations_lt_json = excluded.translations_lt_json,
			updated_at = CURRENT_TIMESTAMP
	`

func resolveEntryFromSources(entry *LexemeEntry, srcMap map[SourceTier]LexemeSource) {
	if len(srcMap) == 0 {
		return
	}
	if userSrc, hasUser := srcMap[SourceUser]; hasUser {
		entry.FullDefinition = userSrc.FullDefinition
		if userSrc.ShortDefinitions != nil {
			entry.ShortDefinitions = append([]string{}, userSrc.ShortDefinitions...)
		} else {
			entry.ShortDefinitions = []string{}
		}
		if userSrc.TranslationsLT != nil {
			entry.TranslationsLT = append([]string{}, userSrc.TranslationsLT...)
		} else {
			entry.TranslationsLT = []string{}
		}
		return
	}

	offlineSrc, hasOffline := srcMap[SourceOffline]
	onlineSrc, hasOnline := srcMap[SourceOnline]

	if hasOffline && strings.TrimSpace(offlineSrc.FullDefinition) != "" {
		entry.FullDefinition = offlineSrc.FullDefinition
	} else if hasOnline && strings.TrimSpace(onlineSrc.FullDefinition) != "" {
		entry.FullDefinition = onlineSrc.FullDefinition
	} else if hasOffline {
		entry.FullDefinition = offlineSrc.FullDefinition
	}

	entry.ShortDefinitions = strlist.MergeUnique(offlineSrc.ShortDefinitions, onlineSrc.ShortDefinitions)
	entry.TranslationsLT = strlist.MergeUnique(offlineSrc.TranslationsLT, onlineSrc.TranslationsLT)
}

func (s *Store) attachSources(entries []LexemeEntry) error {
	if len(entries) == 0 {
		return nil
	}

	placeholders := make([]string, len(entries))
	args := make([]any, len(entries))
	for i := range entries {
		placeholders[i] = "?"
		args[i] = entries[i].ID
	}

	query := fmt.Sprintf(`
		SELECT lexeme_id, source, full_definition, short_definitions_json, translations_lt_json
		FROM lexeme_sources
		WHERE lexeme_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	byLexeme := make(map[int64]map[SourceTier]LexemeSource)
	for rows.Next() {
		var src LexemeSource
		var shortJSON, ltJSON string
		if err := rows.Scan(&src.LexemeID, &src.Source, &src.FullDefinition, &shortJSON, &ltJSON); err != nil {
			return err
		}
		src.ShortDefinitions = decodeStringList(shortJSON)
		src.TranslationsLT = decodeStringList(ltJSON)
		if byLexeme[src.LexemeID] == nil {
			byLexeme[src.LexemeID] = make(map[SourceTier]LexemeSource)
		}
		byLexeme[src.LexemeID][src.Source] = src
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range entries {
		if srcMap, ok := byLexeme[entries[i].ID]; ok {
			resolveEntryFromSources(&entries[i], srcMap)
		}
	}
	return nil
}

func (s *Store) UpsertLexemeSource(lexemeID int64, source SourceTier, content SourceContent) error {
	if _, err := s.getLexemeProgress(lexemeID); err != nil {
		return err
	}
	shortJSON, err := encodeStringList(content.ShortDefinitions)
	if err != nil {
		return err
	}
	ltJSON, err := encodeStringList(content.TranslationsLT)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(upsertLexemeSourceSQL, lexemeID, source, content.FullDefinition, shortJSON, ltJSON)
	return err
}

// AppendOnlineTranslations merges translations into the lexeme's `online`
// source, keeping its other fields and the offline and user sources intact.
// It reports whether any non-blank translation was given; otherwise nothing
// is written.
func (s *Store) AppendOnlineTranslations(lexemeID int64, translations []string) (bool, error) {
	fetched := strlist.MergeUnique(translations)
	if len(fetched) == 0 {
		return false, nil
	}
	sources, err := s.GetLexemeSources(lexemeID)
	if err != nil {
		return false, err
	}
	content := sources[SourceOnline].SourceContent
	content.TranslationsLT = strlist.MergeUnique(content.TranslationsLT, fetched)
	if err := s.UpsertLexemeSource(lexemeID, SourceOnline, content); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) GetLexemeSources(lexemeID int64) (map[SourceTier]LexemeSource, error) {
	if _, err := s.getLexemeProgress(lexemeID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT lexeme_id, source, full_definition, short_definitions_json, translations_lt_json
		FROM lexeme_sources
		WHERE lexeme_id = ?
	`, lexemeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[SourceTier]LexemeSource)
	for rows.Next() {
		var src LexemeSource
		var shortJSON, ltJSON string
		if err := rows.Scan(&src.LexemeID, &src.Source, &src.FullDefinition, &shortJSON, &ltJSON); err != nil {
			return nil, err
		}
		src.ShortDefinitions = decodeStringList(shortJSON)
		src.TranslationsLT = decodeStringList(ltJSON)
		out[src.Source] = src
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListBookVocabulary(bookID int64, filter VocabularyFilter) (VocabularyPage, error) {
	book, err := s.GetBook(bookID)
	if err != nil {
		return VocabularyPage{}, err
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	var whereClauses []string
	var args []any
	whereClauses = append(whereClauses, "bl.book_id = ?")
	args = append(args, bookID)

	if filter.Statuses != nil {
		var statusClauses []string
		for _, rawStatus := range filter.Statuses {
			if clause, ok := lexemeStatusSQL[strings.ToLower(strings.TrimSpace(rawStatus))]; ok {
				statusClauses = append(statusClauses, clause)
			}
		}
		if len(statusClauses) == 0 {
			return VocabularyPage{
				Book:       book,
				Items:      []LexemeEntry{},
				Total:      0,
				Page:       page,
				PageSize:   pageSize,
				TotalPages: 0,
			}, nil
		}
		whereClauses = append(whereClauses, "("+strings.Join(statusClauses, " OR ")+")")
	} else if !filter.IncludeKnown {
		whereClauses = append(whereClauses, "l.known = 0")
	}

	posFilter := strings.ToLower(strings.TrimSpace(filter.POS))
	if posFilter != "" {
		whereClauses = append(whereClauses, "l.pos = ?")
		args = append(args, posFilter)
	}

	searchFilter := strings.ToLower(strings.TrimSpace(filter.Search))
	if searchFilter != "" {
		whereClauses = append(whereClauses, "LOWER(l.lemma) LIKE '%' || ? || '%'")
		args = append(args, searchFilter)
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	var total int
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM book_lexemes bl
		JOIN lexemes l ON l.id = bl.lexeme_id
		WHERE %s
	`, whereSQL)
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return VocabularyPage{}, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	offset := (page - 1) * pageSize
	queryArgs := append(append([]any{}, args...), pageSize, offset)

	dataQuery := fmt.Sprintf(`
		WITH tb AS (
			SELECT COUNT(*) AS total_books FROM books
		),
		lc AS (
			SELECT lexeme_id, COUNT(*) AS books_with_lexeme
			FROM book_lexemes
			GROUP BY lexeme_id
		)
		SELECT
			l.id,
			l.lemma,
			l.pos,
			COALESCE(l.cefr_level, ''),
			max(0.0, 5.0 - l.global_zipf_freq) AS global_rarity,
			ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL)) AS library_idf,
			ln(1.0 + CAST(bl.occurrence_count AS REAL)) AS book_salience,
			(
				max(0.0, 5.0 - l.global_zipf_freq)
				+ ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL))
				+ ln(1.0 + CAST(bl.occurrence_count AS REAL))
			) AS obscurity_score,
			l.global_zipf_freq,
			bl.occurrence_count,
			lc.books_with_lexeme,
			tb.total_books,
			l.full_definition,
			l.short_definitions_json,
			l.translations_lt_json,
			l.mastery_level,
			l.known
		FROM book_lexemes bl
		JOIN lexemes l ON l.id = bl.lexeme_id
		JOIN lc ON lc.lexeme_id = l.id
		CROSS JOIN tb
		WHERE %s
		ORDER BY obscurity_score DESC, l.lemma ASC, l.pos ASC
		LIMIT ? OFFSET ?
	`, whereSQL)

	rows, err := s.db.Query(dataQuery, queryArgs...)
	if err != nil {
		return VocabularyPage{}, err
	}
	defer rows.Close()

	items := make([]LexemeEntry, 0)
	for rows.Next() {
		entry, err := scanLexemeEntry(rows)
		if err != nil {
			return VocabularyPage{}, err
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return VocabularyPage{}, err
	}

	if err := s.attachSources(items); err != nil {
		return VocabularyPage{}, err
	}

	if err := s.attachExcerpts(bookID, items); err != nil {
		return VocabularyPage{}, err
	}

	return VocabularyPage{
		Book:       book,
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *Store) GetLexeme(lexemeID int64) (*LexemeEntry, error) {
	var entry LexemeEntry
	var shortJSON, ltJSON string
	err := s.db.QueryRow(`
		WITH tb AS (
			SELECT COUNT(*) AS total_books FROM books
		),
		lc AS (
			SELECT COUNT(*) AS books_with_lexeme
			FROM book_lexemes
			WHERE lexeme_id = ?
		),
		occ AS (
			SELECT COALESCE(MAX(occurrence_count), 0) AS occurrence_count
			FROM book_lexemes
			WHERE lexeme_id = ?
		)
		SELECT
			l.id,
			l.lemma,
			l.pos,
			COALESCE(l.cefr_level, ''),
			max(0.0, 5.0 - l.global_zipf_freq) AS global_rarity,
			CASE WHEN lc.books_with_lexeme > 0
				THEN ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL))
				ELSE 0.0
			END AS library_idf,
			ln(1.0 + CAST(occ.occurrence_count AS REAL)) AS book_salience,
			(
				max(0.0, 5.0 - l.global_zipf_freq)
				+ CASE WHEN lc.books_with_lexeme > 0
					THEN ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL))
					ELSE 0.0
				END
				+ ln(1.0 + CAST(occ.occurrence_count AS REAL))
			) AS obscurity_score,
			l.global_zipf_freq,
			occ.occurrence_count,
			lc.books_with_lexeme,
			tb.total_books,
			l.full_definition,
			l.short_definitions_json,
			l.translations_lt_json,
			l.mastery_level,
			l.known
		FROM lexemes l
		CROSS JOIN tb
		CROSS JOIN lc
		CROSS JOIN occ
		WHERE l.id = ?
	`, lexemeID, lexemeID, lexemeID).Scan(
		&entry.ID,
		&entry.Lemma,
		&entry.POS,
		&entry.CEFRLevel,
		&entry.GlobalRarity,
		&entry.LibraryIDF,
		&entry.BookSalience,
		&entry.ObscurityScore,
		&entry.GlobalZipfFreq,
		&entry.OccurrenceCount,
		&entry.BooksWithLexeme,
		&entry.TotalBooks,
		&entry.FullDefinition,
		&shortJSON,
		&ltJSON,
		&entry.MasteryLevel,
		&entry.Known,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLexemeNotFound
	}
	if err != nil {
		return nil, err
	}
	entry.ShortDefinitions = decodeStringList(shortJSON)
	entry.TranslationsLT = decodeStringList(ltJSON)
	entry.Excerpts = []Excerpt{}

	entries := []LexemeEntry{entry}
	if err := s.attachSources(entries); err != nil {
		return nil, err
	}
	return &entries[0], nil
}

func (s *Store) PatchLexeme(lexemeID int64, patch LexemePatch) (*LexemeEntry, error) {
	current, err := s.GetLexeme(lexemeID)
	if err != nil {
		return nil, err
	}

	if patch.Known != nil {
		if _, err := s.db.Exec(`UPDATE lexemes SET known = ? WHERE id = ?`, *patch.Known, lexemeID); err != nil {
			return nil, err
		}
	}

	if patch.FullDefinition != nil || patch.ShortDefinitions != nil || patch.TranslationsLT != nil {
		fullDef := current.FullDefinition
		if patch.FullDefinition != nil {
			fullDef = *patch.FullDefinition
		}
		shortDefs := current.ShortDefinitions
		if patch.ShortDefinitions != nil {
			shortDefs = *patch.ShortDefinitions
		}
		transLT := current.TranslationsLT
		if patch.TranslationsLT != nil {
			transLT = *patch.TranslationsLT
		}
		if err := s.UpsertLexemeSource(lexemeID, SourceUser, SourceContent{
			FullDefinition:   fullDef,
			ShortDefinitions: shortDefs,
			TranslationsLT:   transLT,
		}); err != nil {
			return nil, err
		}
	}

	return s.GetLexeme(lexemeID)
}

func (s *Store) getLexemeProgress(lexemeID int64) (*LexemeProgress, error) {
	var st LexemeProgress
	err := s.db.QueryRow(`
		SELECT id, lemma, pos, mastery_level, known, COALESCE(last_quizzed_at, '')
		FROM lexemes
		WHERE id = ?
	`, lexemeID).Scan(&st.ID, &st.Lemma, &st.POS, &st.MasteryLevel, &st.Known, &st.LastQuizzedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLexemeNotFound
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateLexeme(lexemeID int64, known *bool) (*LexemeProgress, error) {
	st, err := s.getLexemeProgress(lexemeID)
	if err != nil {
		return nil, err
	}

	newKnown := st.Known
	if known != nil {
		newKnown = *known
	}

	if _, err := s.db.Exec(
		`UPDATE lexemes SET known = ? WHERE id = ?`,
		newKnown, lexemeID,
	); err != nil {
		return nil, err
	}
	return s.getLexemeProgress(lexemeID)
}

// RecordLexemeAttempt commits the Mastery Level transition for a first
// attempt: +1 (no upper cap) when correct, reset to 0 otherwise. The level
// before the transition is remembered so that a later correction (the user
// edits the lexeme on the reveal card and the wrong answer now matches) can
// replace the transition with pre-attempt level +1 instead of 0 + 1.
func (s *Store) RecordLexemeAttempt(lexemeID int64, firstAttemptCorrect, correction bool) (*LexemeProgress, error) {
	st, err := s.getLexemeProgress(lexemeID)
	if err != nil {
		return nil, err
	}

	baseLevel := st.MasteryLevel
	if correction {
		var pre sql.NullInt64
		if err := s.db.QueryRow(
			`SELECT pre_attempt_mastery_level FROM lexemes WHERE id = ?`, lexemeID,
		).Scan(&pre); err != nil {
			return nil, err
		}
		if pre.Valid {
			baseLevel = int(pre.Int64)
		}
	}

	newMastery := 0
	if firstAttemptCorrect {
		newMastery = baseLevel + 1
	}

	if _, err := s.db.Exec(
		`UPDATE lexemes SET mastery_level = ?, pre_attempt_mastery_level = ?, last_quizzed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newMastery, baseLevel, lexemeID,
	); err != nil {
		return nil, err
	}
	return s.getLexemeProgress(lexemeID)
}

func (s *Store) SelectQuizCandidates(bookID int64, size int) (QuizSession, error) {
	book, err := s.GetBook(bookID)
	if err != nil {
		return QuizSession{}, err
	}

	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}

	rows, err := s.db.Query(`
		WITH tb AS (
			SELECT COUNT(*) AS total_books FROM books
		),
		lc AS (
			SELECT lexeme_id, COUNT(*) AS books_with_lexeme
			FROM book_lexemes
			GROUP BY lexeme_id
		),
		due AS (
			SELECT l.id, `+dueDateSQL+` AS due_date
			FROM lexemes l
		),
		sched AS (
			SELECT
				id,
				due_date,
				-- 0 = Due, 1 = New, 2 = Not Due
				CASE
					WHEN due_date IS NULL THEN 1
					WHEN due_date <= date('now', 'localtime') THEN 0
					ELSE 2
				END AS sched_group
			FROM due
		)
		SELECT
			l.id,
			l.lemma,
			l.pos,
			COALESCE(l.cefr_level, ''),
			max(0.0, 5.0 - l.global_zipf_freq) AS global_rarity,
			ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL)) AS library_idf,
			ln(1.0 + CAST(bl.occurrence_count AS REAL)) AS book_salience,
			(
				max(0.0, 5.0 - l.global_zipf_freq)
				+ ln(1.0 + CAST(tb.total_books AS REAL) / CAST(lc.books_with_lexeme AS REAL))
				+ ln(1.0 + CAST(bl.occurrence_count AS REAL))
			) AS obscurity_score,
			l.global_zipf_freq,
			bl.occurrence_count,
			lc.books_with_lexeme,
			tb.total_books,
			l.full_definition,
			l.short_definitions_json,
			l.translations_lt_json,
			l.mastery_level,
			l.known
		FROM book_lexemes bl
		JOIN lexemes l ON l.id = bl.lexeme_id
		JOIN lc ON lc.lexeme_id = l.id
		JOIN sched s ON s.id = l.id
		CROSS JOIN tb
		WHERE bl.book_id = ? AND l.known = 0
		ORDER BY
			s.sched_group ASC,
			s.due_date ASC,
			CASE WHEN s.sched_group = 0 THEN l.mastery_level ELSE 0 END ASC,
			obscurity_score DESC,
			l.lemma ASC,
			l.pos ASC
		LIMIT ?
	`, bookID, size)
	if err != nil {
		return QuizSession{}, err
	}
	defer rows.Close()

	items := make([]LexemeEntry, 0)
	for rows.Next() {
		entry, err := scanLexemeEntry(rows)
		if err != nil {
			return QuizSession{}, err
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return QuizSession{}, err
	}

	if err := s.attachSources(items); err != nil {
		return QuizSession{}, err
	}

	if err := s.attachExcerpts(bookID, items); err != nil {
		return QuizSession{}, err
	}

	return QuizSession{
		Book:  book,
		Items: items,
	}, nil
}


