package enrich_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"wordhoard/internal/db"
	"wordhoard/internal/enrich"
)

func TestRenderProgressShowsDoneTotalAndPercentage(t *testing.T) {
	view := enrich.RenderProgress(enrich.Progress{Done: 3, Total: 12, Batch: 1, Batches: 4, Status: "batch 1/4 saved"})
	for _, want := range []string{"3 / 12", "25%", "batch 1/4 saved"} {
		if !strings.Contains(view, want) {
			t.Errorf("progress view missing %q:\n%s", want, view)
		}
	}
	if empty := enrich.RenderProgress(enrich.Progress{}); !strings.Contains(empty, "0 / 0") || !strings.Contains(empty, "100%") {
		t.Errorf("empty progress view should read 0 / 0 at 100%%, got:\n%s", empty)
	}
}

func TestProgressModelUpdatesViewAndQuitsWhenFinished(t *testing.T) {
	m := enrich.NewProgressModel()
	next, _ := m.Update(enrich.Progress{Done: 1, Total: 2, Status: "batch 1/2 saved"})
	if !strings.Contains(next.View(), "1 / 2") || !strings.Contains(next.View(), "50%") {
		t.Errorf("view after progress:\n%s", next.View())
	}
	final, cmd := next.Update(enrich.DoneMsg{Summary: enrich.Summary{Total: 2, Translated: 2}})
	if cmd == nil {
		t.Fatalf("expected a quit command after DoneMsg")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg after DoneMsg")
	}
	if !strings.Contains(final.View(), "2 translated") {
		t.Errorf("final view should include summary:\n%s", final.View())
	}
}

func TestRunTUIPrintsPlainSummaryWhenNotATerminal(t *testing.T) {
	store, _ := seedStore(t, []db.LexemeInput{
		{Lemma: "laconic", POS: "adjective"},
		{Lemma: "obdurate", POS: "adjective"},
	})
	tr := &fakeTranslator{answers: map[string][]string{"laconic": {"lakoniškas"}}}

	var out bytes.Buffer
	summary, err := enrich.RunTUI(context.Background(), store, tr, enrich.Options{BatchSize: 1}, &out, false)
	if err != nil {
		t.Fatalf("RunTUI: %v", err)
	}
	if summary.Translated != 1 {
		t.Errorf("unexpected summary: %+v", summary)
	}
	text := out.String()
	for _, want := range []string{"2 / 2", "100%", "1 translated", "1 without results", "0 failed"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Errorf("non-TTY output must not contain ANSI escapes:\n%q", text)
	}
}
