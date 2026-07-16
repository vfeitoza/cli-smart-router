package domain

import "strings"

// Intent is the classified high-level purpose of a user prompt, produced by the
// Prompt Analyzer layer. It is a string-based enum so it renders readably in
// debug logs and JSON without a separate lookup table.
//
// IntentUnknown is the zero value and represents an ambiguous or empty prompt
// that matched no known signal. Callers (future Complexity/Policy layers) should
// treat IntentUnknown as low-confidence and defer to their own fallbacks.
type Intent string

const (
	// IntentUnknown means no intent signal matched (ambiguous or empty prompt).
	IntentUnknown Intent = ""
	// IntentPlanning covers architecture, design, and high-level planning tasks.
	IntentPlanning Intent = "planning"
	// IntentCoding covers writing or implementing code.
	IntentCoding Intent = "coding"
	// IntentReview covers reviewing or evaluating existing code.
	IntentReview Intent = "review"
	// IntentTesting covers writing tests or improving coverage.
	IntentTesting Intent = "testing"
	// IntentDebug covers diagnosing errors, bugs, and failures.
	IntentDebug Intent = "debug"
	// IntentSecurity covers security analysis and vulnerability work.
	IntentSecurity Intent = "security"
	// IntentDocumentation covers writing docs, docstrings, and READMEs.
	IntentDocumentation Intent = "documentation"
	// IntentPerformance covers optimization and performance analysis.
	IntentPerformance Intent = "performance"
)

// String returns the enum's string form.
func (i Intent) String() string {
	if i == IntentUnknown {
		return "unknown"
	}
	return string(i)
}

// Valid reports whether the intent is a known, non-unknown classification.
func (i Intent) Valid() bool {
	switch i {
	case IntentPlanning, IntentCoding, IntentReview, IntentTesting,
		IntentDebug, IntentSecurity, IntentDocumentation, IntentPerformance:
		return true
	default:
		return false
	}
}

// ParseIntent returns a known intent for a routing override value.
func ParseIntent(value string) Intent {
	switch Intent(strings.ToLower(strings.TrimSpace(value))) {
	case IntentPlanning:
		return IntentPlanning
	case IntentCoding:
		return IntentCoding
	case IntentReview:
		return IntentReview
	case IntentTesting:
		return IntentTesting
	case IntentDebug:
		return IntentDebug
	case IntentSecurity:
		return IntentSecurity
	case IntentDocumentation:
		return IntentDocumentation
	case IntentPerformance:
		return IntentPerformance
	default:
		return IntentUnknown
	}
}

// RouterTaskIntent extracts a valid [router-task: intent] marker from a prompt.
func RouterTaskIntent(prompt string) Intent {
	return routerMarkerIntent(prompt, "[router-task:", ParseIntent)
}

// RouterAgentIntent maps known OpenCode subagent names to their task intent.
func RouterAgentIntent(agent string) Intent {
	switch strings.ToLower(strings.TrimSpace(agent)) {
	case "planner":
		return IntentPlanning
	case "implementer":
		return IntentCoding
	case "reviewer":
		return IntentReview
	default:
		return IntentUnknown
	}
}

// RouterAgentTagIntent extracts a valid [router-agent: name] marker from a prompt.
func RouterAgentTagIntent(prompt string) Intent {
	return routerMarkerIntent(prompt, "[router-agent:", RouterAgentIntent)
}

func routerMarkerIntent(prompt, marker string, parse func(string) Intent) Intent {
	lower := strings.ToLower(prompt)
	for {
		start := strings.Index(lower, marker)
		if start < 0 {
			return IntentUnknown
		}
		value := lower[start+len(marker):]
		if end := strings.IndexByte(value, ']'); end >= 0 {
			if intent := parse(value[:end]); intent.Valid() {
				return intent
			}
		}
		lower = value
	}
}

// intentSignal maps a lowercase keyword to the intent it implies and a weight.
// Keywords are matched as substrings against the lowercased prompt, covering both
// Portuguese and English without a full NLP pipeline, mirroring capabilitySignals.
type intentSignal struct {
	keyword string
	intent  Intent
	weight  int
}

// intentSignals is intentionally small and conservative: it should only catch
// clear, common intents. Ambiguous prompts are expected to match nothing and
// yield IntentUnknown so downstream layers can defer to their fallbacks.
var intentSignals = []intentSignal{
	// Planning
	{"arquitetura", IntentPlanning, 2},
	{"architecture", IntentPlanning, 2},
	{"design de sistema", IntentPlanning, 2},
	{"system design", IntentPlanning, 2},
	{"roadmap", IntentPlanning, 2},
	{"planeje", IntentPlanning, 2},
	{"planejar", IntentPlanning, 2},
	{"plano", IntentPlanning, 1},
	{"planning", IntentPlanning, 2},
	{"estratégia", IntentPlanning, 1},
	{"estrategia", IntentPlanning, 1},
	{"strategy", IntentPlanning, 1},

	// Coding
	{"implemente", IntentCoding, 2},
	{"implement", IntentCoding, 2},
	{"escreva código", IntentCoding, 2},
	{"escreva codigo", IntentCoding, 2},
	{"write code", IntentCoding, 2},
	{"código", IntentCoding, 1},
	{"codigo", IntentCoding, 1},
	{"code", IntentCoding, 1},
	{"função", IntentCoding, 1},
	{"funcao", IntentCoding, 1},
	{"function", IntentCoding, 1},
	{"refatore", IntentCoding, 1},
	{"refactor", IntentCoding, 1},

	// Review
	{"code review", IntentReview, 3},
	{"pull request", IntentReview, 2},
	{"revise o código", IntentReview, 2},
	{"revise o codigo", IntentReview, 2},
	{"revisão de código", IntentReview, 2},
	{"revisao de codigo", IntentReview, 2},
	{"revis", IntentReview, 1},
	{"review", IntentReview, 1},
	{"avalie o código", IntentReview, 2},
	{"avalie o codigo", IntentReview, 2},

	// Testing
	{"teste unitário", IntentTesting, 2},
	{"teste unitario", IntentTesting, 2},
	{"unit test", IntentTesting, 2},
	{"cobertura de teste", IntentTesting, 2},
	{"test coverage", IntentTesting, 2},
	{"coverage", IntentTesting, 1},
	{"cobertura", IntentTesting, 1},
	{"teste", IntentTesting, 1},
	{"testes", IntentTesting, 1},
	{"tdd", IntentTesting, 2},

	// Debug
	{"stacktrace", IntentDebug, 3},
	{"stack trace", IntentDebug, 3},
	{"exception", IntentDebug, 2},
	{"debug", IntentDebug, 2},
	{"corrigir bug", IntentDebug, 2},
	{"fix bug", IntentDebug, 2},
	{"bug", IntentDebug, 1},
	{"erro", IntentDebug, 1},
	{"error", IntentDebug, 1},
	{"falha", IntentDebug, 1},

	// Security
	{"vulnerabilidade", IntentSecurity, 3},
	{"vulnerability", IntentSecurity, 3},
	{"segurança", IntentSecurity, 2},
	{"seguranca", IntentSecurity, 2},
	{"security", IntentSecurity, 2},
	{"owasp", IntentSecurity, 3},
	{"injection", IntentSecurity, 2},
	{"xss", IntentSecurity, 2},
	{"cve", IntentSecurity, 2},

	// Documentation
	{"documentação", IntentDocumentation, 2},
	{"documentacao", IntentDocumentation, 2},
	{"documentation", IntentDocumentation, 2},
	{"documente", IntentDocumentation, 2},
	{"docstring", IntentDocumentation, 2},
	{"readme", IntentDocumentation, 2},
	{"comentário", IntentDocumentation, 1},
	{"comentario", IntentDocumentation, 1},

	// Performance
	{"performance", IntentPerformance, 2},
	{"desempenho", IntentPerformance, 2},
	{"otimiz", IntentPerformance, 2},
	{"optimize", IntentPerformance, 2},
	{"optimization", IntentPerformance, 2},
	{"latência", IntentPerformance, 1},
	{"latencia", IntentPerformance, 1},
	{"latency", IntentPerformance, 1},
	{"throughput", IntentPerformance, 1},
	{"gargalo", IntentPerformance, 2},
	{"bottleneck", IntentPerformance, 2},
	{"memory leak", IntentPerformance, 2},
}

// intentPriority breaks weight ties deterministically. Earlier intents win when
// two categories accumulate the same total weight, so specific/high-stakes
// intents (Security, Debug) are preferred over generic ones (Coding, Planning).
var intentPriority = []Intent{
	IntentSecurity,
	IntentDebug,
	IntentTesting,
	IntentReview,
	IntentPerformance,
	IntentDocumentation,
	IntentCoding,
	IntentPlanning,
}

// PromptAnalysis is the Prompt Analyzer output contract consumed by downstream
// layers (Complexity/Policy). Matched reports whether any intent signal fired so
// callers can distinguish a confident classification from an ambiguous prompt.
type PromptAnalysis struct {
	Intent  Intent
	Matched bool
}

// DetectIntent classifies the high-level intent of a prompt using conservative
// keyword matching. It sums signal weights per intent and returns the highest,
// breaking ties via intentPriority. An empty or ambiguous prompt returns
// IntentUnknown.
func DetectIntent(prompt string) Intent {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if lower == "" {
		return IntentUnknown
	}
	scores := make(map[Intent]int)
	for _, signal := range intentSignals {
		if strings.Contains(lower, signal.keyword) {
			scores[signal.intent] += signal.weight
		}
	}
	if len(scores) == 0 {
		return IntentUnknown
	}
	best := IntentUnknown
	bestScore := 0
	for _, intent := range intentPriority {
		score := scores[intent]
		if score > bestScore {
			best = intent
			bestScore = score
		}
	}
	return best
}

// AnalyzePrompt runs the Prompt Analyzer and returns the structured result used
// by later layers. It performs no I/O and has no host or ABI dependencies.
func AnalyzePrompt(prompt string) PromptAnalysis {
	intent := DetectIntent(prompt)
	return PromptAnalysis{Intent: intent, Matched: intent.Valid()}
}
