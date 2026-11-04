package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

var (
	ErrBookNotFound   = errors.New("book not found")
	ErrLexemeNotFound = errors.New("lexeme not found")
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
	PromptForm       string
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
	PromptForm       string    `json:"prompt_form"`
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
	Page             int
	PageSize         int
	POS              string
	Search           string
	IncludeKnown     bool
	StatusesProvided bool
	Statuses         []string
}

type LexemeStatus struct {
	ID            int64  `json:"id"`
	Lemma         string `json:"lemma"`
	POS           string `json:"pos"`
	MasteryLevel  int    `json:"mastery_level"`
	Known         bool   `json:"known"`
	LastQuizzedAt string `json:"last_quizzed_at,omitempty"`
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

func (s *Store) GetBook(bookID int64) (*Book, error) {
	var b Book
	var m0, m1, m2, m3, m4, m5 int
	err := s.db.QueryRow(`
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
		WHERE b.id = ?
		GROUP BY b.id
	`, bookID).Scan(
		&b.ID, &b.Title, &b.Author, &b.Filename, &b.FileSize, &b.SHA256, &b.ImportedAt,
		&b.TotalLexemes, &b.KnownCount, &b.MasteredCount,
		&m0, &m1, &m2, &m3, &m4, &m5,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBookNotFound
	}
	if err != nil {
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
			lemma, pos, prompt_form, cefr_level, global_zipf_freq,
			full_definition, short_definitions_json, translations_lt_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(lemma, pos) DO UPDATE SET
			prompt_form = excluded.prompt_form,
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

	for _, item := range items {
		shortDefs := item.ShortDefinitions
		if shortDefs == nil {
			shortDefs = []string{}
		}
		shortJSON, err := json.Marshal(shortDefs)
		if err != nil {
			return err
		}

		transLT := item.TranslationsLT
		if transLT == nil {
			transLT = []string{}
		}
		ltJSON, err := json.Marshal(transLT)
		if err != nil {
			return err
		}

		if _, err := upsertLexemeStmt.Exec(
			item.Lemma,
			item.POS,
			item.PromptForm,
			item.CEFRLevel,
			item.GlobalZipfFreq,
			item.FullDefinition,
			string(shortJSON),
			string(ltJSON),
		); err != nil {
			return err
		}

		var lexemeID int64
		if err := selectLexemeIDStmt.QueryRow(item.Lemma, item.POS).Scan(&lexemeID); err != nil {
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
		&entry.PromptForm,
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
	if err := json.Unmarshal([]byte(shortJSON), &entry.ShortDefinitions); err != nil || entry.ShortDefinitions == nil {
		entry.ShortDefinitions = []string{}
	}
	if err := json.Unmarshal([]byte(ltJSON), &entry.TranslationsLT); err != nil || entry.TranslationsLT == nil {
		entry.TranslationsLT = []string{}
	}
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

	if filter.StatusesProvided {
		var statusClauses []string
		for _, rawStatus := range filter.Statuses {
			switch strings.ToLower(strings.TrimSpace(rawStatus)) {
			case "unseen":
				statusClauses = append(statusClauses, "(l.known = 0 AND l.mastery_level = 0)")
			case "learning":
				statusClauses = append(statusClauses, "(l.known = 0 AND l.mastery_level BETWEEN 1 AND 4)")
			case "mastered":
				statusClauses = append(statusClauses, "(l.known = 0 AND l.mastery_level >= 5)")
			case "known":
				statusClauses = append(statusClauses, "(l.known = 1)")
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
			l.prompt_form,
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

	if err := s.attachExcerpts(bookID, items); err != nil {
		return VocabularyPage{}, err
	}

	return VocabularyPage{
		Book:       book,
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *Store) getLexemeStatus(lexemeID int64) (*LexemeStatus, error) {
	var st LexemeStatus
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

func (s *Store) UpdateLexeme(lexemeID int64, known *bool) (*LexemeStatus, error) {
	st, err := s.getLexemeStatus(lexemeID)
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
	return s.getLexemeStatus(lexemeID)
}

func (s *Store) RecordLexemeAttempt(lexemeID int64, firstAttemptCorrect bool) (*LexemeStatus, error) {
	st, err := s.getLexemeStatus(lexemeID)
	if err != nil {
		return nil, err
	}

	newMastery := 0
	if firstAttemptCorrect {
		newMastery = st.MasteryLevel + 1
		if newMastery > 5 {
			newMastery = 5
		}
	}

	if _, err := s.db.Exec(
		`UPDATE lexemes SET mastery_level = ?, last_quizzed_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newMastery, lexemeID,
	); err != nil {
		return nil, err
	}
	return s.getLexemeStatus(lexemeID)
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
		)
		SELECT
			l.id,
			l.lemma,
			l.pos,
			l.prompt_form,
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
		WHERE bl.book_id = ? AND l.known = 0 AND l.mastery_level < 5
		ORDER BY
			l.mastery_level ASC,
			CASE
				WHEN l.last_quizzed_at IS NULL OR datetime(l.last_quizzed_at) <= datetime('now', '-15 minutes') THEN 0
				ELSE 1
			END ASC,
			obscurity_score DESC,
			COALESCE(l.last_quizzed_at, '') ASC,
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

	if err := s.attachExcerpts(bookID, items); err != nil {
		return QuizSession{}, err
	}

	return QuizSession{
		Book:  book,
		Items: items,
	}, nil
}


