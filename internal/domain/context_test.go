package domain

import "testing"

func TestAnalyzeContextExplicitFilesAndTools(t *testing.T) {
	in := ContextInput{
		Prompt:       "refatore",
		HistoryTurns: 4,
		Files:        []string{"main.go", "router.go", "main.go"}, // dup ignored
		Tools:        []string{"read", "edit", "read"},            // dup ignored
	}
	got := AnalyzeContext(in)
	if got.FileCount != 2 {
		t.Fatalf("expected 2 files, got %d", got.FileCount)
	}
	if got.ToolCount != 2 {
		t.Fatalf("expected 2 tools, got %d", got.ToolCount)
	}
	if got.HistoryTurns != 4 {
		t.Fatalf("expected 4 turns, got %d", got.HistoryTurns)
	}
	if got.Language != "go" {
		t.Fatalf("expected go, got %q", got.Language)
	}
}

func TestAnalyzeContextDetectsLanguageFromExtensions(t *testing.T) {
	got := AnalyzeContext(ContextInput{Files: []string{"a.py", "b.py", "c.go"}})
	if got.Language != "python" {
		t.Fatalf("expected python (majority), got %q", got.Language)
	}
	if got.FileCount != 3 {
		t.Fatalf("expected 3 files, got %d", got.FileCount)
	}
}

func TestAnalyzeContextExtractsFilesFromBody(t *testing.T) {
	body := "Please update internal/domain/router.go and cmd/plugin/main.go"
	got := AnalyzeContext(ContextInput{Body: body})
	if got.FileCount != 2 {
		t.Fatalf("expected 2 files from body, got %d", got.FileCount)
	}
	if got.Language != "go" {
		t.Fatalf("expected go, got %q", got.Language)
	}
	if got.ContextSize != len(body) {
		t.Fatalf("expected context size %d, got %d", len(body), got.ContextSize)
	}
}

func TestAnalyzeContextDiffSize(t *testing.T) {
	body := "diff --git a/x.go b/x.go\n@@ -1 +1 @@\n-old line\n+new line here\n"
	got := AnalyzeContext(ContextInput{Body: body})
	if got.DiffSize == 0 {
		t.Fatalf("expected non-zero diff size, got %d", got.DiffSize)
	}
	// "-old line" (8) + "+new line here" (14) = 22
	if got.DiffSize != len("-old line")+len("+new line here") {
		t.Fatalf("unexpected diff size %d", got.DiffSize)
	}
}

func TestAnalyzeContextNoDiff(t *testing.T) {
	got := AnalyzeContext(ContextInput{Body: "just a plain sentence with no diff"})
	if got.DiffSize != 0 {
		t.Fatalf("expected zero diff size, got %d", got.DiffSize)
	}
}

func TestAnalyzeContextEmpty(t *testing.T) {
	got := AnalyzeContext(ContextInput{})
	if got.FileCount != 0 || got.ToolCount != 0 || got.ContextSize != 0 ||
		got.HistoryTurns != 0 || got.DiffSize != 0 || got.Language != "" {
		t.Fatalf("expected zero-value signals, got %+v", got)
	}
}

func TestAnalyzeContextFallsBackToPromptForSize(t *testing.T) {
	got := AnalyzeContext(ContextInput{Prompt: "hello world"})
	if got.ContextSize != len("hello world") {
		t.Fatalf("expected size from prompt, got %d", got.ContextSize)
	}
}
