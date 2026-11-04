package db

import (
	"encoding/json"
	"strings"
)

// EnrichFilter scopes the selection of Lexemes that need online Translations.
// Zero values mean "all books" and "no limit".
type EnrichFilter struct {
	BookID int64
	Limit  int
}

// ListEnrichCandidates returns imported Lexemes whose resolved Lithuanian
// Translations are empty, resolved as described in ADR-0001. Lexemes with a
// `user` source are never candidates: the user's values always win, so an
// online translation would not be displayed.
func (s *Store) ListEnrichCandidates(filter EnrichFilter) ([]LexemeEntry, error) {
	var b strings.Builder
	args := []any{}
	b.WriteString(`
		SELECT l.id, l.lemma, l.pos, l.prompt_form, COALESCE(l.cefr_level, ''),
			l.full_definition, l.short_definitions_json, l.translations_lt_json,
			l.mastery_level, l.known
		FROM lexemes l
		WHERE EXISTS (
			SELECT 1 FROM book_lexemes bl
			WHERE bl.lexeme_id = l.id`)
	if filter.BookID > 0 {
		b.WriteString(` AND bl.book_id = ?`)
		args = append(args, filter.BookID)
	}
	b.WriteString(`
		)
		AND NOT EXISTS (
			SELECT 1 FROM lexeme_sources ls
			WHERE ls.lexeme_id = l.id
			AND (ls.source = 'user' OR json_array_length(ls.translations_lt_json) > 0)
		)
		AND (
			EXISTS (SELECT 1 FROM lexeme_sources ls WHERE ls.lexeme_id = l.id)
			OR json_array_length(l.translations_lt_json) = 0
		)
		ORDER BY l.id`)
	if filter.Limit > 0 {
		b.WriteString(` LIMIT ?`)
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []LexemeEntry{}
	for rows.Next() {
		var e LexemeEntry
		var shortJSON, ltJSON string
		if err := rows.Scan(&e.ID, &e.Lemma, &e.POS, &e.PromptForm, &e.CEFRLevel,
			&e.FullDefinition, &shortJSON, &ltJSON, &e.MasteryLevel, &e.Known); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(shortJSON), &e.ShortDefinitions); err != nil || e.ShortDefinitions == nil {
			e.ShortDefinitions = []string{}
		}
		if err := json.Unmarshal([]byte(ltJSON), &e.TranslationsLT); err != nil || e.TranslationsLT == nil {
			e.TranslationsLT = []string{}
		}
		e.Excerpts = []Excerpt{}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachSources(entries); err != nil {
		return nil, err
	}
	return entries, nil
}
