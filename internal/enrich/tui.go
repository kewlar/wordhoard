package enrich

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	"wordhoard/internal/db"
	"wordhoard/internal/translate"
)

const (
	title    = "Wordhoard Enrich"
	barWidth = 30
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	barStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// RenderProgress renders a plain-text progress line: bar, done / total,
// percentage and the current batch status.
func RenderProgress(p Progress) string {
	return renderProgress(p, func(s string) string { return s })
}

func renderProgress(p Progress, styleBar func(string) string) string {
	ratio := 1.0
	if p.Total > 0 {
		ratio = float64(p.Done) / float64(p.Total)
	}
	filled := int(ratio * barWidth)
	bar := styleBar(strings.Repeat("█", filled)) + strings.Repeat("░", barWidth-filled)
	line := fmt.Sprintf("[%s] %d / %d (%d%%)", bar, p.Done, p.Total, int(ratio*100))
	if p.Status != "" {
		line += "  " + p.Status
	}
	return line
}

func renderSummary(s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Summary: %d lexemes, %d translated, %d without results, %d failed\n",
		s.Total, s.Translated, s.Empty, s.Failed)
	for _, err := range s.Errors {
		fmt.Fprintf(&b, "  error: %v\n", err)
	}
	return b.String()
}

// DoneMsg tells the progress model that the run has finished.
type DoneMsg struct {
	Summary Summary
	Err     error
}

// ProgressModel is the BubbleTea model behind the live progress bar.
type ProgressModel struct {
	progress Progress
	summary  Summary
	err      error
	finished bool
	updates  <-chan Progress
	done     <-chan DoneMsg
}

func NewProgressModel() ProgressModel {
	return ProgressModel{progress: Progress{Status: "selecting lexemes"}}
}

func (m ProgressModel) Init() tea.Cmd {
	return m.wait()
}

func (m ProgressModel) wait() tea.Cmd {
	if m.updates == nil {
		return nil
	}
	updates, done := m.updates, m.done
	return func() tea.Msg {
		if p, ok := <-updates; ok {
			return p
		}
		return <-done
	}
}

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case Progress:
		m.progress = msg
		return m, m.wait()
	case DoneMsg:
		m.summary, m.err, m.finished = msg.Summary, msg.Err, true
		return m, tea.Quit
	}
	return m, nil
}

func (m ProgressModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")
	b.WriteString(renderProgress(m.progress, func(s string) string { return barStyle.Render(s) }))
	b.WriteString("\n")
	if m.finished {
		b.WriteString("\n")
		b.WriteString(renderSummary(m.summary))
	}
	return b.String()
}

// RunTUI runs enrichment with a live BubbleTea progress bar when out is a
// terminal and noTUI is false; otherwise it prints one plain progress line per
// batch followed by the summary.
func RunTUI(ctx context.Context, store *db.Store, tr translate.Translator, opts Options, out io.Writer, noTUI bool) (Summary, error) {
	if out == nil {
		out = os.Stdout
	}
	isTTY := false
	if f, ok := out.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		isTTY = true
	}

	if noTUI || !isTTY {
		fmt.Fprintln(out, title)
		summary, err := Run(ctx, store, tr, opts, func(p Progress) {
			fmt.Fprintln(out, RenderProgress(p))
		})
		fmt.Fprint(out, renderSummary(summary))
		return summary, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	updates := make(chan Progress, 16)
	done := make(chan DoneMsg, 1)
	go func() {
		summary, err := Run(ctx, store, tr, opts, func(p Progress) { updates <- p })
		close(updates)
		done <- DoneMsg{Summary: summary, Err: err}
	}()

	model := NewProgressModel()
	model.updates, model.done = updates, done
	final, err := tea.NewProgram(model, tea.WithOutput(out)).Run()
	if err != nil {
		return Summary{}, err
	}
	fm := final.(ProgressModel)
	if !fm.finished {
		// User quit early: stop the run and wait for persisted work to settle.
		cancel()
		for range updates {
		}
		res := <-done
		return res.Summary, context.Canceled
	}
	return fm.summary, fm.err
}
