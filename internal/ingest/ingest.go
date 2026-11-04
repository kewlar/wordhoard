package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/tsawler/prose/v3"

	"wordhoard/internal/db"
	"wordhoard/internal/dictionary"
	"wordhoard/internal/epub"
)

type Option func(*config)

type config struct {
	dict *dictionary.Dictionary
}

func WithDictionary(d *dictionary.Dictionary) Option {
	return func(c *config) {
		c.dict = d
	}
}


type Status string

const (
	StatusPending    Status = "Pending"
	StatusParsing   Status = "Parsing"   // dedup check and metadata
	StatusReading   Status = "Reading"   // unzipping chapters and paragraphs
	StatusAnalyzing Status = "Analyzing" // tagging words and looking them up, chapter by chapter
	StatusSaving    Status = "Saving"    // writing the book, lexemes and excerpts to SQLite
	StatusSkipped   Status = "Skipped"
	StatusDone      Status = "Done"
	StatusError     Status = "Error"
)

// Event reports a file's progress. While Analyzing, Progress/Total count
// chapters processed out of the book's total.
type Event struct {
	Index       int
	Path        string
	Status      Status
	Title       string
	Author      string
	LexemeCount int
	Progress    int
	Total       int
	Err         error
}

type FileResult struct {
	Path        string
	Status      Status
	Title       string
	Author      string
	LexemeCount int
	Progress    int
	Total       int
	Err         error
}

// taggerModel returns prose's part-of-speech model, decoded once per process.
// Without it, every prose.NewDocument call re-decodes the ~5 MB model.
var taggerModel = sync.OnceValue(func() *prose.Model {
	doc, err := prose.NewDocument("", prose.WithExtraction(false), prose.WithSegmentation(false))
	if err != nil {
		panic(fmt.Sprintf("ingest: failed to load part-of-speech model: %v", err))
	}
	return doc.Model
})

type Summary struct {
	TotalFiles int
	Imported   int
	Skipped    int
	Failed     int
	Files      []FileResult
}

func ResolveFiles(patterns []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, pattern := range patterns {
		expanded := expandHome(pattern)
		matches, err := filepath.Glob(expanded)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			// Check if pattern is a literal path or directory
			info, statErr := os.Stat(expanded)
			if statErr == nil {
				if info.IsDir() {
					dirMatches, _ := filepath.Glob(filepath.Join(expanded, "*.epub"))
					matches = append(matches, dirMatches...)
				} else {
					matches = append(matches, expanded)
				}
			}
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			if !strings.EqualFold(filepath.Ext(m), ".epub") {
				continue
			}
			abs, err := filepath.Abs(m)
			if err != nil {
				abs = m
			}
			if _, ok := seen[abs]; !ok {
				seen[abs] = struct{}{}
				files = append(files, m)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func Run(store *db.Store, patterns []string, onProgress func(Event), opts ...Option) (Summary, error) {
	cfg := &config{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	dict := cfg.dict
	if dict == nil {
		dict = dictionary.Default()
	}

	files, err := ResolveFiles(patterns)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		TotalFiles: len(files),
		Files:      make([]FileResult, len(files)),
	}
	for i, path := range files {
		summary.Files[i] = FileResult{
			Path:   path,
			Status: StatusPending,
		}
	}

	for i, path := range files {
		res := processFile(store, dict, i, path, onProgress)
		summary.Files[i] = res
		switch res.Status {
		case StatusDone:
			summary.Imported++
		case StatusSkipped:
			summary.Skipped++
		case StatusError:
			summary.Failed++
		}
	}

	return summary, nil
}

func processFile(store *db.Store, dict *dictionary.Dictionary, index int, path string, onProgress func(Event)) FileResult {

	emit := func(ev Event) {
		if onProgress != nil {
			onProgress(ev)
		}
	}

	emit(Event{Index: index, Path: path, Status: StatusParsing})

	info, err := os.Stat(path)
	if err != nil {
		emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
		return FileResult{Path: path, Status: StatusError, Err: err}
	}
	fileSize := info.Size()

	// Step 1 of two-step deduplication: check if any book with this file_size exists
	hasSize, err := store.HasBookWithSize(fileSize)
	if err != nil {
		emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
		return FileResult{Path: path, Status: StatusError, Err: err}
	}

	var hashHex string
	if hasSize {
		// Step 2: compute SHA-256 and check if (file_size, sha256) matches an existing Book
		hashHex, err = computeSHA256(path)
		if err != nil {
			emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
			return FileResult{Path: path, Status: StatusError, Err: err}
		}
		existing, err := store.FindBookBySizeAndHash(fileSize, hashHex)
		if err != nil {
			emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
			return FileResult{Path: path, Status: StatusError, Err: err}
		}
		if existing != nil {
			if existing.Filename != path {
				_ = store.UpdateBookFilename(existing.ID, path)
			}
			emit(Event{
				Index:  index,
				Path:   path,
				Status: StatusSkipped,
				Title:  existing.Title,
				Author: existing.Author,
			})
			return FileResult{
				Path:   path,
				Status: StatusSkipped,
				Title:  existing.Title,
				Author: existing.Author,
			}
		}
	}

	meta, err := epub.ReadMetadata(path)
	if err != nil {
		emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
		return FileResult{Path: path, Status: StatusError, Err: err}
	}

	if hashHex == "" {
		hashHex, err = computeSHA256(path)
		if err != nil {
			emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
			return FileResult{Path: path, Status: StatusError, Err: err}
		}
	}

	stage := func(status Status, progress, total int) {
		emit(Event{
			Index:    index,
			Path:     path,
			Status:   status,
			Title:    meta.Title,
			Author:   meta.Author,
			Progress: progress,
			Total:    total,
		})
	}

	stage(StatusReading, 0, 0)
	chapters, err := epub.ReadChapters(path)
	if err != nil {
		emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
		return FileResult{Path: path, Status: StatusError, Err: err}
	}

	stage(StatusAnalyzing, 0, len(chapters))
	extracted := extractBookLexemes(chapters, dict, func(done int) {
		stage(StatusAnalyzing, done, len(chapters))
	})

	stage(StatusSaving, 0, 0)

	book, _, err := store.CreateBook(db.BookInput{
		Title:    meta.Title,
		Author:   meta.Author,
		Filename: path,
		FileSize: fileSize,
		SHA256:   hashHex,
	})
	if err != nil {
		emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
		return FileResult{Path: path, Status: StatusError, Err: err}
	}

	if len(extracted) > 0 {
		if err := store.SaveBookLexemes(book.ID, extracted); err != nil {
			emit(Event{Index: index, Path: path, Status: StatusError, Err: err})
			return FileResult{Path: path, Status: StatusError, Err: err}
		}
	}

	lexemeCount := len(extracted)
	emit(Event{
		Index:       index,
		Path:        path,
		Status:      StatusDone,
		Title:       meta.Title,
		Author:      meta.Author,
		LexemeCount: lexemeCount,
	})
	return FileResult{
		Path:        path,
		Status:      StatusDone,
		Title:       meta.Title,
		Author:      meta.Author,
		LexemeCount: lexemeCount,
	}
}

var quoteReplacer = strings.NewReplacer(
	"“", "\"",
	"”", "\"",
	"„", "\"",
	"‟", "\"",
	"‘", "'",
	"’", "'",
	"‚", "'",
	"‛", "'",
	"—", " -- ",
	"–", " - ",
	"…", "...",
	"\u00a0", " ",
)

type lexemeKey struct {
	lemma string
	pos   dictionary.POS
}

const (
	maxExcerptsPerLexeme = 5
	maxExcerptLength     = 400
	excerptRadius        = 180
)

func normalizeExcerptText(s string) string {
	return strings.TrimSpace(quoteReplacer.Replace(s))
}

// extractBookLexemes calls onChapterDone (if non-nil) with the number of
// chapters processed so far after each chapter.
func extractBookLexemes(chapters []epub.Chapter, dict *dictionary.Dictionary, onChapterDone func(done int)) []db.LexemeInput {
	counts := make(map[lexemeKey]int)
	results := make(map[lexemeKey]*dictionary.LookupResult)
	excerpts := make(map[lexemeKey][]db.ExcerptInput)
	model := taggerModel()

	for chIdx, ch := range chapters {
		if chIdx > 0 && onChapterDone != nil {
			onChapterDone(chIdx)
		}
		for _, para := range ch.Paragraphs {
			clean := normalizeExcerptText(para)
			if clean == "" {
				continue
			}
			doc, err := prose.NewDocument(
				clean,
				prose.WithExtraction(false),
				prose.WithSegmentation(false),
				prose.UsingModel(model),
			)
			if err != nil {
				continue
			}
			searchPos := 0
			for _, tok := range doc.Tokens() {
				tokStart := -1
				if tok.Text != "" {
					if idx := strings.Index(clean[searchPos:], tok.Text); idx >= 0 {
						tokStart = searchPos + idx
						searchPos = tokStart + len(tok.Text)
					} else if idx := strings.Index(clean, tok.Text); idx >= 0 {
						tokStart = idx
					}
				}

				res, ok := dict.Lookup(tok.Text, tok.Tag)
				if !ok || res.IsCoreVocabulary {
					continue
				}
				key := lexemeKey{lemma: res.Lemma, pos: res.POS}
				counts[key]++
				if _, exists := results[key]; !exists {
					results[key] = res
				}
				if len(excerpts[key]) < maxExcerptsPerLexeme && tokStart >= 0 {
					ex := buildExcerpt(ch.Title, clean, tokStart, tokStart+len(tok.Text))
					excerpts[key] = append(excerpts[key], ex)
				}
			}
		}
	}
	if len(chapters) > 0 && onChapterDone != nil {
		onChapterDone(len(chapters))
	}

	out := make([]db.LexemeInput, 0, len(results))
	for key, res := range results {
		out = append(out, db.LexemeInput{
			Lemma:            res.Lemma,
			POS:              string(res.POS),
			PromptForm:       res.PromptForm,
			CEFRLevel:        res.CEFRLevel,
			GlobalZipfFreq:   res.GlobalZipfFreq,
			FullDefinition:   res.FullDefinition,
			ShortDefinitions: res.ShortDefinitions,
			TranslationsLT:   res.TranslationsLT,
			OccurrenceCount:  counts[key],
			Excerpts:         excerpts[key],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lemma != out[j].Lemma {
			return out[i].Lemma < out[j].Lemma
		}
		return out[i].POS < out[j].POS
	})
	return out
}

func buildExcerpt(chapter, clean string, start, end int) db.ExcerptInput {
	surface := clean[start:end]
	if len(clean) <= maxExcerptLength {
		return db.ExcerptInput{
			Chapter:        chapter,
			SurfaceWord:    surface,
			Text:           clean,
			HighlightStart: start,
			HighlightEnd:   end,
		}
	}

	winStart := start - excerptRadius
	if winStart < 0 {
		winStart = 0
	} else {
		// Snap forward to the next space so we don't slice mid-word or mid-rune
		if spaceIdx := strings.IndexByte(clean[winStart:start], ' '); spaceIdx >= 0 {
			winStart = winStart + spaceIdx + 1
		}
	}

	winEnd := end + excerptRadius
	if winEnd > len(clean) {
		winEnd = len(clean)
	} else {
		// Snap backward to the previous space so we don't slice mid-word or mid-rune
		if spaceIdx := strings.LastIndexByte(clean[end:winEnd], ' '); spaceIdx >= 0 {
			winEnd = end + spaceIdx
		}
	}

	var b strings.Builder
	offsetShift := 0
	if winStart > 0 {
		b.WriteString("...")
		offsetShift = 3
	}
	b.WriteString(clean[winStart:winEnd])
	if winEnd < len(clean) {
		b.WriteString("...")
	}

	snippet := b.String()
	hStart := (start - winStart) + offsetShift
	hEnd := hStart + len(surface)

	return db.ExcerptInput{
		Chapter:        chapter,
		SurfaceWord:    surface,
		Text:           snippet,
		HighlightStart: hStart,
		HighlightEnd:   hEnd,
	}
}


func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BubbleTea TUI model & runner

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	skippedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	activeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

type tuiDoneMsg struct {
	summary Summary
	err     error
}

type tuiModel struct {
	files    []FileResult
	eventsCh <-chan Event
	doneCh   <-chan tuiDoneMsg
	summary  Summary
	err      error
	finished bool
}

func (m tuiModel) Init() tea.Cmd {
	return waitForEvent(m.eventsCh, m.doneCh)
}

func waitForEvent(eventsCh <-chan Event, doneCh <-chan tuiDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-eventsCh:
			if ok {
				return ev
			}
			return <-doneCh
		case done := <-doneCh:
			return done
		}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case Event:
		if msg.Index >= 0 && msg.Index < len(m.files) {
			m.files[msg.Index] = FileResult{
				Path:        msg.Path,
				Status:      msg.Status,
				Title:       msg.Title,
				Author:      msg.Author,
				LexemeCount: msg.LexemeCount,
				Progress:    msg.Progress,
				Total:       msg.Total,
				Err:         msg.Err,
			}
		}
		return m, waitForEvent(m.eventsCh, m.doneCh)
	case tuiDoneMsg:
		m.summary = msg.summary
		if len(msg.summary.Files) == len(m.files) {
			m.files = msg.summary.Files
		}
		m.err = msg.err
		m.finished = true
		return m, tea.Quit
	}
	return m, nil
}

func (m tuiModel) View() string {
	return renderDashboard(m.files, m.summary, m.finished)
}

func renderDashboard(files []FileResult, summary Summary, finished bool) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Wordhoard Ingester"))
	b.WriteString("\n\n")

	if len(files) == 0 {
		b.WriteString("No matching .epub files found.\n")
		return b.String()
	}

	for _, f := range files {
		name := filepath.Base(f.Path)
		statusStr := formatStatus(f.Status)
		details := ""
		if f.Title != "" {
			details = fmt.Sprintf(" — %s (%s)", f.Title, f.Author)
		}
		if f.Status == StatusDone {
			details += fmt.Sprintf(" [%d lexemes]", f.LexemeCount)
		} else if f.Status == StatusAnalyzing && f.Total > 0 {
			details += fmt.Sprintf(" [chapter %d/%d]", f.Progress, f.Total)
		} else if f.Status == StatusError && f.Err != nil {
			details += fmt.Sprintf(" [%v]", f.Err)
		}
		fmt.Fprintf(&b, "  [%s] %s%s\n", statusStr, name, details)
	}

	if finished {
		fmt.Fprintf(
			&b,
			"\nSummary: %d total, %d imported, %d skipped, %d failed\n",
			summary.TotalFiles,
			summary.Imported,
			summary.Skipped,
			summary.Failed,
		)
	}
	return b.String()
}

func formatStatus(s Status) string {
	switch s {
	case StatusDone:
		return doneStyle.Render(string(s))
	case StatusSkipped:
		return skippedStyle.Render(string(s))
	case StatusParsing, StatusReading, StatusAnalyzing, StatusSaving:
		return activeStyle.Render(string(s))
	case StatusError:
		return errorStyle.Render(string(s))
	default:
		return string(s)
	}
}

func RunTUI(store *db.Store, patterns []string, out io.Writer, opts ...Option) (Summary, error) {
	if out == nil {
		out = os.Stdout
	}
	files, err := ResolveFiles(patterns)
	if err != nil {
		return Summary{}, err
	}

	initial := make([]FileResult, len(files))
	for i, p := range files {
		initial[i] = FileResult{Path: p, Status: StatusPending}
	}

	isTTY := false
	if f, ok := out.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		isTTY = true
	}

	if !isTTY {
		summary, err := Run(store, patterns, nil, opts...)
		fmt.Fprint(out, renderDashboard(summary.Files, summary, true))
		return summary, err
	}

	eventsCh := make(chan Event, 64)
	doneCh := make(chan tuiDoneMsg, 1)

	go func() {
		summary, runErr := Run(store, patterns, func(ev Event) {
			eventsCh <- ev
		}, opts...)
		close(eventsCh)
		doneCh <- tuiDoneMsg{summary: summary, err: runErr}
	}()


	model := tuiModel{
		files:    initial,
		eventsCh: eventsCh,
		doneCh:   doneCh,
	}

	p := tea.NewProgram(model, tea.WithOutput(out))
	finalModel, err := p.Run()
	if err != nil {
		return Summary{}, err
	}
	if fm, ok := finalModel.(tuiModel); ok {
		return fm.summary, fm.err
	}
	return Summary{}, nil
}
