// Package enrich bulk-fetches online Lithuanian Translations for imported
// Lexemes whose resolved translations are empty, persisting each batch to the
// `online` Lexeme Source (ADR-0001).
package enrich

import (
	"context"
	"fmt"
	"strings"

	"wordhoard/internal/db"
	"wordhoard/internal/translate"
)

const DefaultBatchSize = 20

type Options struct {
	BookID    int64
	Limit     int
	BatchSize int
}

// Progress is reported once before the first batch and after every batch.
type Progress struct {
	Done       int
	Total      int
	Batch      int
	Batches    int
	Translated int
	Failed     int
	Status     string
}

type Summary struct {
	Total      int
	Translated int
	Empty      int
	Failed     int
	Errors     []error
}

// Run selects candidates, translates them in batches and persists each batch
// before requesting the next. Translator errors fail only the affected batch;
// store errors abort the run.
func Run(ctx context.Context, store *db.Store, tr translate.Translator, opts Options, onProgress func(Progress)) (Summary, error) {
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	candidates, err := store.ListEnrichCandidates(db.EnrichFilter{BookID: opts.BookID, Limit: opts.Limit})
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Total: len(candidates)}
	batches := (len(candidates) + batchSize - 1) / batchSize
	p := Progress{Total: len(candidates), Batches: batches, Status: "starting"}
	onProgress(p)

	for start, batch := 0, 1; start < len(candidates); start, batch = start+batchSize, batch+1 {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		chunk := candidates[start:min(start+batchSize, len(candidates))]
		p.Batch = batch

		reqs := make([]translate.TranslateRequest, len(chunk))
		for i, lex := range chunk {
			reqs[i] = translate.TranslateRequest{
				ID:             lex.ID,
				Lemma:          lex.Lemma,
				POS:            lex.POS,
				PromptForm:     lex.PromptForm,
				FullDefinition: lex.FullDefinition,
			}
		}

		results, trErr := tr.TranslateBatch(ctx, reqs)
		if trErr != nil {
			summary.Failed += len(chunk)
			summary.Errors = append(summary.Errors, fmt.Errorf("batch %d: %w", batch, trErr))
			p.Status = fmt.Sprintf("batch %d/%d failed: %v", batch, batches, trErr)
		} else {
			for _, lex := range chunk {
				translated, err := persistOnline(store, lex.ID, results[lex.ID])
				if err != nil {
					return summary, err
				}
				if translated {
					summary.Translated++
				} else {
					summary.Empty++
				}
			}
			p.Status = fmt.Sprintf("batch %d/%d saved", batch, batches)
		}
		p.Done += len(chunk)
		p.Translated = summary.Translated
		p.Failed = summary.Failed
		onProgress(p)
	}
	return summary, nil
}

// persistOnline merges fetched translations into the lexeme's `online` source,
// keeping any other online fields intact. It never touches offline or user rows.
func persistOnline(store *db.Store, lexemeID int64, fetched []string) (bool, error) {
	clean := make([]string, 0, len(fetched))
	for _, s := range fetched {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return false, nil
	}
	sources, err := store.GetLexemeSources(lexemeID)
	if err != nil {
		return false, err
	}
	online := sources["online"]
	merged := append(append([]string{}, online.TranslationsLT...), clean...)
	if err := store.UpsertLexemeSource(lexemeID, "online", online.FullDefinition, online.ShortDefinitions, dedupe(merged)); err != nil {
		return false, err
	}
	return true, nil
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}
