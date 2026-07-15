package application

import (
	"testing"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

// benchConfig mirrors a realistic configs/smart-model-router.yaml: a full
// candidate matrix plus a set of routing rules. It is normalized once so the
// benchmark measures the decision pipeline, not config parsing.
func benchConfig() domain.Config {
	cfg := domain.Config{
		Enabled:      true,
		VirtualModel: "router:auto",
		Strategy:     "decision_engine",
		Preference:   "balanced",
		Models: []domain.CandidateConfig{
			{Provider: "claude", Model: "claude-opus-4-8", Capabilities: []string{"reasoning", "architecture", "planning", "analysis"}, Cost: "very_high", Quality: "highest"},
			{Provider: "claude", Model: "claude-sonnet-5", Capabilities: []string{"coding", "reasoning", "review", "refactor"}, Cost: "high", Quality: "high"},
			{Provider: "claude", Model: "claude-haiku-4-5-20251001", Capabilities: []string{"classify", "summarize", "translate", "fast"}, Cost: "low", Quality: "medium"},
			{Provider: "codex", Model: "gpt-5.5", Capabilities: []string{"coding", "reasoning", "agents", "tools"}, Cost: "very_high", Quality: "highest"},
			{Provider: "codex", Model: "gpt-5.4", Capabilities: []string{"coding", "reasoning", "tools", "review", "tests"}, Cost: "high", Quality: "high"},
			{Provider: "codex", Model: "gpt-5.4-mini", Capabilities: []string{"classify", "summarize", "simple_coding", "fast"}, Cost: "low", Quality: "medium"},
		},
		Routes: []domain.RouteRule{
			{When: domain.RouteCondition{Task: "coding", Language: "go", Complexity: "low"}, Provider: "codex", Model: "gpt-5.4-mini"},
			{When: domain.RouteCondition{Task: "coding", Complexity: "high"}, Provider: "claude", Model: "claude-sonnet-5"},
			{When: domain.RouteCondition{Task: "security"}, Provider: "claude", Model: "claude-opus-4-8"},
			{When: domain.RouteCondition{Task: "review"}, Provider: "codex", Model: "gpt-5.4"},
			{When: domain.RouteCondition{Task: "planning"}, Provider: "claude", Model: "claude-opus-4-8"},
			{When: domain.RouteCondition{Task: "testing"}, Provider: "codex", Model: "gpt-5.4"},
		},
	}
	return cfg.Normalize()
}

// runPipeline replicates the exact Decision Engine flow the plugin runs per
// request (cmd/plugin/main.go buildRouteFacts + decisionEngineRoute), minus the
// cgo host boundary: Prompt -> Context -> Complexity -> Policy Engine -> Router.
func runPipeline(cfg domain.Config, req infrastructure.ModelRouteRequest, candidates []domain.Candidate) domain.RouteDecision {
	prompt := infrastructure.ExtractUserPrompt(req.Body)
	ctx := domain.AnalyzeContext(domain.ContextInput{Prompt: prompt, Body: string(req.Body)})
	complexity := domain.AssessComplexity(domain.ComplexityInputFromSignals(prompt, ctx, nil, 0))
	facts := domain.RouteFacts{
		Task:            domain.DetectIntent(prompt),
		Language:        ctx.Language,
		ComplexityScore: complexity.Score,
		ComplexityTier:  complexity.MinCostTier,
		FileCount:       ctx.FileCount,
		HasDiff:         ctx.DiffSize > 0,
		Stream:          req.Stream,
	}
	chain := domain.EvaluateRoutesRanked(cfg.Routes, facts, candidates, req.AvailableProviders)
	if len(chain) == 0 {
		return domain.RouteDecision{Handled: false}
	}
	router := PolicyRouter{Config: cfg}
	return router.Forward(chain[0], req.AvailableProviders)
}

func benchRequest(body string) infrastructure.ModelRouteRequest {
	return infrastructure.ModelRouteRequest{
		RequestedModel:     "router:auto",
		Body:               []byte(body),
		AvailableProviders: []string{"claude", "codex"},
	}
}

var benchScenarios = []struct {
	name string
	body string
}{
	{"short_coding", `{"messages":[{"role":"user","content":"write a Go function to reverse a string"}]}`},
	{"security", `{"messages":[{"role":"user","content":"audit this handler for SQL injection vulnerabilities"}]}`},
	{"planning", `{"messages":[{"role":"user","content":"design the architecture for a multi-tenant billing system"}]}`},
	{"no_match", `{"messages":[{"role":"user","content":"olá, tudo bem?"}]}`},
	{"large_diff", `{"messages":[{"role":"user","content":"review this change:\n` + largeDiff() + `"}]}`},
}

// largeDiff builds a ~4KB unified-diff-like body to stress the context/diff analyzers.
func largeDiff() string {
	line := "+ added line of code that changes behavior in file.go\\n- removed old line\\n"
	out := "diff --git a/file.go b/file.go\\n"
	for i := 0; i < 60; i++ {
		out += line
	}
	return out
}

func BenchmarkDecisionEngine(b *testing.B) {
	cfg := benchConfig()
	candidates := make([]domain.Candidate, 0, len(cfg.Models))
	for _, item := range cfg.Models {
		candidates = append(candidates, domain.CandidateFromConfig(item))
	}
	for _, sc := range benchScenarios {
		req := benchRequest(sc.body)
		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = runPipeline(cfg, req, candidates)
			}
		})
	}
}

// BenchmarkDecisionEngineTarget asserts the 5ms-per-decision goal: it fails the
// benchmark if the average decision time exceeds the target.
func BenchmarkDecisionEngineTarget(b *testing.B) {
	const targetNsPerOp = 5_000_000 // 5 ms
	cfg := benchConfig()
	candidates := make([]domain.Candidate, 0, len(cfg.Models))
	for _, item := range cfg.Models {
		candidates = append(candidates, domain.CandidateFromConfig(item))
	}
	req := benchRequest(benchScenarios[2].body) // planning: full pipeline, rule match
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = runPipeline(cfg, req, candidates)
	}
	b.StopTimer()
	nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	if nsPerOp > targetNsPerOp {
		b.Fatalf("decision took %.0f ns/op (%.3f ms), exceeds 5ms target", nsPerOp, nsPerOp/1e6)
	}
}
