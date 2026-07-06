package domain

import "strings"

// capabilitySignal maps lowercase prompt keywords to the capability tags they imply.
// Keywords are matched as substrings against the lowercased prompt text, so both
// Portuguese and English signals are covered without a full NLP pipeline.
type capabilitySignal struct {
	keyword      string
	capabilities []string
}

// capabilitySignals is intentionally small and conservative: it should only catch
// clear, common intents. Ambiguous prompts are expected to yield few or no matches,
// which naturally lowers scoring confidence and defers to the classifier.
var capabilitySignals = []capabilitySignal{
	{"stacktrace", []string{"coding", "review"}},
	{"stack trace", []string{"coding", "review"}},
	{"bug", []string{"coding", "review"}},
	{"erro", []string{"coding", "review"}},
	{"error", []string{"coding", "review"}},
	{"falha", []string{"coding", "review"}},
	{"exception", []string{"coding", "review"}},
	{"refatora", []string{"refactor", "coding"}},
	{"refactor", []string{"refactor", "coding"}},
	{"melhorar código", []string{"refactor", "coding"}},
	{"melhorar codigo", []string{"refactor", "coding"}},
	{"arquitetura", []string{"architecture", "planning"}},
	{"architecture", []string{"architecture", "planning"}},
	{"design", []string{"architecture", "planning"}},
	{"planeje", []string{"planning", "architecture"}},
	{"plan", []string{"planning"}},
	{"explique", []string{"reasoning", "analysis"}},
	{"explain", []string{"reasoning", "analysis"}},
	{"analise", []string{"analysis", "reasoning"}},
	{"analyze", []string{"analysis", "reasoning"}},
	{"compare", []string{"analysis", "reasoning"}},
	{"como funciona", []string{"reasoning", "analysis"}},
	{"resuma", []string{"summarize"}},
	{"resumo", []string{"summarize"}},
	{"summarize", []string{"summarize"}},
	{"sumarize", []string{"summarize"}},
	{"traduza", []string{"translate"}},
	{"tradução", []string{"translate"}},
	{"translate", []string{"translate"}},
	{"classifique", []string{"classify"}},
	{"classify", []string{"classify"}},
	{"categorize", []string{"classify"}},
	{"teste", []string{"tests", "coding"}},
	{"unit test", []string{"tests", "coding"}},
	{"cobertura", []string{"tests", "coding"}},
	{"coverage", []string{"tests", "coding"}},
	{"agente", []string{"agents", "tools"}},
	{"agent", []string{"agents", "tools"}},
	{"ferramenta", []string{"tools"}},
	{"tool", []string{"tools"}},
	{"código", []string{"coding"}},
	{"codigo", []string{"coding"}},
	{"code", []string{"coding"}},
	{"documenta", []string{"documentation", "writing"}},
	{"documentation", []string{"documentation", "writing"}},
	{"escreva", []string{"writing"}},
	{"write", []string{"writing"}},
	{"revis", []string{"review"}},
	{"review", []string{"review"}},
}

// longPromptThreshold is the character count above which a prompt is considered to
// need long-context handling.
const longPromptThreshold = 1200

// InferCapabilities returns the capability tags implied by the prompt text using
// simple keyword matching. Results are deduplicated but not ordered by importance.
// An empty or ambiguous prompt returns an empty slice, which callers should treat
// as low-confidence signal.
func InferCapabilities(prompt string) []string {
	lower := strings.ToLower(prompt)
	seen := make(map[string]struct{})
	var out []string
	add := func(cap string) {
		if _, ok := seen[cap]; ok {
			return
		}
		seen[cap] = struct{}{}
		out = append(out, cap)
	}
	for _, signal := range capabilitySignals {
		if strings.Contains(lower, signal.keyword) {
			for _, cap := range signal.capabilities {
				add(cap)
			}
		}
	}
	if len(strings.TrimSpace(prompt)) > longPromptThreshold {
		add("long_context")
	}
	return out
}
