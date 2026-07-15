package domain

import "strings"

// ComplexityAssessment is the Complexity Analyzer output: a normalized score in
// [0,100] plus the derived tier the request needs. Downstream layers (Policy /
// Model Router) use Score and MinCostTier to size the routing decision without
// re-deriving complexity themselves.
type ComplexityAssessment struct {
	// Score is the overall complexity in [0,100].
	Score int
	// MinCostTier is the minimum cost tier the request warrants (0=low..3=very_high),
	// aligned with costRank so Policy can gate candidates consistently.
	MinCostTier int
	// Level is a coarse bucket derived from Score: 0=trivial, 1=simple, 2=moderate, 3=complex.
	Level int
}

// ComplexityInput carries the eight weighted factors the analyzer scores. It is
// pure data (no host/ABI types) so the domain layer stays testable. Callers can
// build it directly or via ComplexityInputFromSignals.
type ComplexityInput struct {
	PromptLength int // characters of the user prompt
	FileCount    int // distinct files referenced
	HistoryTurns int // conversation messages
	ContextSize  int // characters of the analyzed context
	ToolCount    int // distinct tools/functions referenced
	TokenCount   int // token estimate for the context
	DiffSize     int // characters of diff content
	DirCount     int // distinct directories referenced
}

// complexityFactor describes one scored dimension: a raw value, the value at
// which it saturates (contributes its full weight), and its weight. Weights sum
// to 100 so the aggregate score is naturally bounded to [0,100].
type complexityFactor struct {
	value      int
	saturateAt int
	weight     float64
}

// AssessComplexity scores the eight factors and returns a bounded assessment.
//
// Each factor contributes weight * min(value/saturateAt, 1). This makes every
// dimension monotonic (more of it never lowers the score) and saturating (a
// single huge dimension cannot exceed its weight, so no factor alone dominates).
// Weights encode how strongly each dimension signals that a request needs a more
// capable, higher-cost model:
//
//   - ContextSize (20): the strongest signal; large contexts demand long-context,
//     higher-tier models.
//   - TokenCount (18): a token estimate corroborates raw size and cost pressure.
//   - FileCount (15): more files imply cross-file reasoning and coordination.
//   - DiffSize (12): larger diffs mean more code to understand and change safely.
//   - PromptLength (10): longer instructions usually carry more requirements.
//   - HistoryTurns (10): deeper conversations accumulate state and nuance.
//   - DirCount (8): spread across directories implies broader architectural scope.
//   - ToolCount (7): more tools imply more orchestration, but weakly correlates
//     with reasoning depth, so it carries the least weight.
func AssessComplexity(in ComplexityInput) ComplexityAssessment {
	factors := []complexityFactor{
		{in.ContextSize, 16000, 20},
		{in.TokenCount, 6000, 18},
		{in.FileCount, 12, 15},
		{in.DiffSize, 4000, 12},
		{in.PromptLength, 1200, 10},
		{in.HistoryTurns, 20, 10},
		{in.DirCount, 8, 8},
		{in.ToolCount, 10, 7},
	}

	total := 0.0
	for _, f := range factors {
		total += f.weight * saturatingRatio(f.value, f.saturateAt)
	}

	score := int(total + 0.5) // round to nearest
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return ComplexityAssessment{
		Score:       score,
		MinCostTier: tierForScore(score),
		Level:       levelForScore(score),
	}
}

// ComplexityInputFromSignals derives a ComplexityInput from the Prompt/Context
// analyzers plus a token estimate, filling factors not carried by ContextSignals
// (PromptLength, TokenCount, DirCount) from the prompt and file list.
func ComplexityInputFromSignals(prompt string, ctx ContextSignals, files []string, tokenCount int) ComplexityInput {
	if tokenCount <= 0 {
		tokenCount = estimateTokens(ctx.ContextSize)
	}
	return ComplexityInput{
		PromptLength: len(strings.TrimSpace(prompt)),
		FileCount:    ctx.FileCount,
		HistoryTurns: ctx.HistoryTurns,
		ContextSize:  ctx.ContextSize,
		ToolCount:    ctx.ToolCount,
		TokenCount:   tokenCount,
		DiffSize:     ctx.DiffSize,
		DirCount:     countDirectories(files),
	}
}

// saturatingRatio returns min(value/saturateAt, 1) clamped to [0,1].
func saturatingRatio(value, saturateAt int) float64 {
	if value <= 0 || saturateAt <= 0 {
		return 0
	}
	ratio := float64(value) / float64(saturateAt)
	if ratio > 1 {
		return 1
	}
	return ratio
}

// estimateTokens approximates token count from character length (~4 chars/token).
func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return chars / 4
}

// countDirectories counts distinct parent directories among file paths.
func countDirectories(files []string) int {
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		idx := strings.LastIndex(file, "/")
		if idx <= 0 {
			continue
		}
		seen[strings.ToLower(file[:idx])] = struct{}{}
	}
	return len(seen)
}

// tierForScore maps a complexity score to the minimum cost tier (aligned with costRank).
func tierForScore(score int) int {
	switch {
	case score >= 75:
		return 3 // very_high
	case score >= 50:
		return 2 // high
	case score >= 25:
		return 1 // medium
	default:
		return 0 // low
	}
}

// levelForScore maps a complexity score to a coarse level bucket.
func levelForScore(score int) int {
	switch {
	case score >= 75:
		return 3 // complex
	case score >= 50:
		return 2 // moderate
	case score >= 25:
		return 1 // simple
	default:
		return 0 // trivial
	}
}
