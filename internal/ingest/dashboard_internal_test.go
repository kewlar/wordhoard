package ingest

import (
	"strings"
	"testing"
)

func TestRenderDashboardShowsChapterProgressWhileAnalyzing(t *testing.T) {
	files := []FileResult{{Path: "/books/novel.epub", Status: StatusAnalyzing, Title: "Novel", Author: "Someone", Progress: 12, Total: 40}}
	out := renderDashboard(files, Summary{}, false)
	if !strings.Contains(out, "chapter 12/40") {
		t.Errorf("expected chapter progress in dashboard, got:\n%s", out)
	}
}
