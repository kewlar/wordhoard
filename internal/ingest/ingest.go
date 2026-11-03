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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	"ebook-vocab/internal/db"
	"ebook-vocab/internal/epub"
)

type Status string

const (
	StatusPending    Status = "Pending"
	StatusParsing    Status = "Parsing"
	StatusExtracting Status = "Extracting"
	StatusSkipped    Status = "Skipped"
	StatusDone       Status = "Done"
	StatusError      Status = "Error"
)

type Event struct {
	Index        int
	Path         string
	Status       Status
	Title        string
	Author       string
	LexemeCount  int
	Err          error
}

type FileResult struct {
	Path        string
	Status      Status
	Title       string
	Author      string
	LexemeCount int
	Err         error
}

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

func Run(store *db.Store, patterns []string, onProgress func(Event)) (Summary, error) {
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
		res := processFile(store, i, path, onProgress)
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

func processFile(store *db.Store, index int, path string, onProgress func(Event)) FileResult {
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

	emit(Event{
		Index:  index,
		Path:   path,
		Status: StatusExtracting,
		Title:  meta.Title,
		Author: meta.Author,
	})

	_, _, err = store.CreateBook(db.BookInput{
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

	emit(Event{
		Index:       index,
		Path:        path,
		Status:      StatusDone,
		Title:       meta.Title,
		Author:      meta.Author,
		LexemeCount: 0,
	})
	return FileResult{
		Path:        path,
		Status:      StatusDone,
		Title:       meta.Title,
		Author:      meta.Author,
		LexemeCount: 0,
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
	b.WriteString(titleStyle.Render("Ebook Vocabulary Ingester"))
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
	case StatusParsing, StatusExtracting:
		return activeStyle.Render(string(s))
	case StatusError:
		return errorStyle.Render(string(s))
	default:
		return string(s)
	}
}

func RunTUI(store *db.Store, patterns []string, out io.Writer) (Summary, error) {
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
		summary, err := Run(store, patterns, nil)
		fmt.Fprint(out, renderDashboard(summary.Files, summary, true))
		return summary, err
	}

	eventsCh := make(chan Event, 64)
	doneCh := make(chan tuiDoneMsg, 1)

	go func() {
		summary, runErr := Run(store, patterns, func(ev Event) {
			eventsCh <- ev
		})
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
