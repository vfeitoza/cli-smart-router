package domain

import "testing"

func TestAssessComplexityEmptyIsZero(t *testing.T) {
	got := AssessComplexity(ComplexityInput{})
	if got.Score != 0 || got.MinCostTier != 0 || got.Level != 0 {
		t.Fatalf("expected zero assessment, got %+v", got)
	}
}

func TestAssessComplexityBoundedTo100(t *testing.T) {
	// Every factor far above saturation must cap the score at 100.
	got := AssessComplexity(ComplexityInput{
		PromptLength: 100000,
		FileCount:    500,
		HistoryTurns: 500,
		ContextSize:  1000000,
		ToolCount:    500,
		TokenCount:   500000,
		DiffSize:     500000,
		DirCount:     500,
	})
	if got.Score != 100 {
		t.Fatalf("expected score 100, got %d", got.Score)
	}
	if got.MinCostTier != 3 || got.Level != 3 {
		t.Fatalf("expected top tier/level, got tier=%d level=%d", got.MinCostTier, got.Level)
	}
}

func TestAssessComplexityMonotonicInEachFactor(t *testing.T) {
	base := ComplexityInput{ContextSize: 2000, FileCount: 1}
	baseScore := AssessComplexity(base).Score

	more := base
	more.FileCount = 6
	if AssessComplexity(more).Score < baseScore {
		t.Fatalf("adding files must not lower score")
	}

	moreCtx := base
	moreCtx.ContextSize = 8000
	if AssessComplexity(moreCtx).Score <= baseScore {
		t.Fatalf("larger context must raise score")
	}
}

func TestAssessComplexitySingleFactorCannotDominate(t *testing.T) {
	// ContextSize has the highest weight (20); alone it cannot exceed ~20.
	got := AssessComplexity(ComplexityInput{ContextSize: 10000000})
	if got.Score > 20 {
		t.Fatalf("single factor exceeded its weight: score=%d", got.Score)
	}
}

func TestTierAndLevelThresholds(t *testing.T) {
	cases := []struct {
		score int
		tier  int
		level int
	}{
		{0, 0, 0},
		{24, 0, 0},
		{25, 1, 1},
		{49, 1, 1},
		{50, 2, 2},
		{74, 2, 2},
		{75, 3, 3},
		{100, 3, 3},
	}
	for _, c := range cases {
		if got := tierForScore(c.score); got != c.tier {
			t.Fatalf("score %d: expected tier %d, got %d", c.score, c.tier, got)
		}
		if got := levelForScore(c.score); got != c.level {
			t.Fatalf("score %d: expected level %d, got %d", c.score, c.level, got)
		}
	}
}

func TestComplexityInputFromSignals(t *testing.T) {
	ctx := ContextSignals{
		FileCount:    3,
		ContextSize:  4000,
		ToolCount:    2,
		HistoryTurns: 5,
		DiffSize:     800,
	}
	files := []string{"internal/domain/a.go", "internal/app/b.go", "cmd/c.go"}
	in := ComplexityInputFromSignals("refatore o roteamento", ctx, files, 0)

	if in.PromptLength != len("refatore o roteamento") {
		t.Fatalf("unexpected prompt length %d", in.PromptLength)
	}
	if in.TokenCount != 4000/4 {
		t.Fatalf("expected token estimate %d, got %d", 4000/4, in.TokenCount)
	}
	if in.DirCount != 3 {
		t.Fatalf("expected 3 directories, got %d", in.DirCount)
	}
	if in.FileCount != 3 || in.ContextSize != 4000 || in.DiffSize != 800 {
		t.Fatalf("signals not mapped: %+v", in)
	}
}

func TestCountDirectories(t *testing.T) {
	files := []string{"a/b/c.go", "a/b/d.go", "a/e.go", "root.go", "A/B/c.go"}
	// a/b, a, A/b(dup of a/b lowercased) => 2 distinct
	if got := countDirectories(files); got != 2 {
		t.Fatalf("expected 2 directories, got %d", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	if estimateTokens(0) != 0 {
		t.Fatalf("expected 0 tokens for empty")
	}
	if estimateTokens(400) != 100 {
		t.Fatalf("expected 100 tokens, got %d", estimateTokens(400))
	}
}
