package domain

import "testing"

func TestDetectIntentPlanning(t *testing.T) {
	for _, prompt := range []string{
		"Preciso planejar a arquitetura do sistema",
		"Help me design the system architecture",
		"crie um roadmap do produto",
	} {
		if got := DetectIntent(prompt); got != IntentPlanning {
			t.Fatalf("prompt %q: expected planning, got %q", prompt, got)
		}
	}
}

func TestDetectIntentCoding(t *testing.T) {
	for _, prompt := range []string{
		"implemente uma função de soma",
		"write code for a REST handler",
		"refatore este código",
	} {
		if got := DetectIntent(prompt); got != IntentCoding {
			t.Fatalf("prompt %q: expected coding, got %q", prompt, got)
		}
	}
}

func TestDetectIntentReview(t *testing.T) {
	for _, prompt := range []string{
		"faça um code review deste diff",
		"revise o código do pull request",
		"please review this function",
	} {
		if got := DetectIntent(prompt); got != IntentReview {
			t.Fatalf("prompt %q: expected review, got %q", prompt, got)
		}
	}
}

func TestDetectIntentTesting(t *testing.T) {
	for _, prompt := range []string{
		"escreva um teste unitário",
		"write a unit test for this",
		"melhore a cobertura de teste",
	} {
		if got := DetectIntent(prompt); got != IntentTesting {
			t.Fatalf("prompt %q: expected testing, got %q", prompt, got)
		}
	}
}

func TestDetectIntentDebug(t *testing.T) {
	for _, prompt := range []string{
		"analise este stacktrace",
		"there is a bug causing an exception",
		"corrigir bug de produção",
	} {
		if got := DetectIntent(prompt); got != IntentDebug {
			t.Fatalf("prompt %q: expected debug, got %q", prompt, got)
		}
	}
}

func TestDetectIntentSecurity(t *testing.T) {
	for _, prompt := range []string{
		"encontre vulnerabilidades de segurança",
		"check for SQL injection and XSS",
		"is this OWASP compliant?",
	} {
		if got := DetectIntent(prompt); got != IntentSecurity {
			t.Fatalf("prompt %q: expected security, got %q", prompt, got)
		}
	}
}

func TestDetectIntentDocumentation(t *testing.T) {
	for _, prompt := range []string{
		"escreva a documentação da API",
		"add a docstring to this function",
		"generate the README",
	} {
		if got := DetectIntent(prompt); got != IntentDocumentation {
			t.Fatalf("prompt %q: expected documentation, got %q", prompt, got)
		}
	}
}

func TestDetectIntentPerformance(t *testing.T) {
	for _, prompt := range []string{
		"otimize o desempenho desta query",
		"reduce the latency of this endpoint",
		"there is a memory leak causing a bottleneck",
	} {
		if got := DetectIntent(prompt); got != IntentPerformance {
			t.Fatalf("prompt %q: expected performance, got %q", prompt, got)
		}
	}
}

func TestDetectIntentEmptyPromptIsUnknown(t *testing.T) {
	if got := DetectIntent(""); got != IntentUnknown {
		t.Fatalf("expected unknown for empty prompt, got %q", got)
	}
	if got := DetectIntent("   "); got != IntentUnknown {
		t.Fatalf("expected unknown for blank prompt, got %q", got)
	}
}

func TestDetectIntentAmbiguousPromptIsUnknown(t *testing.T) {
	if got := DetectIntent("olá, tudo bem com você hoje?"); got != IntentUnknown {
		t.Fatalf("expected unknown for ambiguous prompt, got %q", got)
	}
}

func TestDetectIntentTiebreakPrefersSecurity(t *testing.T) {
	// Prompt mixes debug and security signals; security has higher priority.
	prompt := "corrigir bug de vulnerabilidade de segurança"
	if got := DetectIntent(prompt); got != IntentSecurity {
		t.Fatalf("expected security to win tiebreak, got %q", got)
	}
}

func TestAnalyzePromptReportsMatched(t *testing.T) {
	analysis := AnalyzePrompt("implemente uma função")
	if analysis.Intent != IntentCoding {
		t.Fatalf("expected coding, got %q", analysis.Intent)
	}
	if !analysis.Matched {
		t.Fatalf("expected matched true for recognized prompt")
	}

	ambiguous := AnalyzePrompt("bom dia")
	if ambiguous.Intent != IntentUnknown || ambiguous.Matched {
		t.Fatalf("expected unknown/unmatched for ambiguous prompt, got %+v", ambiguous)
	}
}

func TestIntentStringAndValid(t *testing.T) {
	if IntentUnknown.String() != "unknown" {
		t.Fatalf("expected unknown string, got %q", IntentUnknown.String())
	}
	if IntentCoding.String() != "coding" {
		t.Fatalf("expected coding string, got %q", IntentCoding.String())
	}
	if IntentUnknown.Valid() {
		t.Fatalf("unknown must not be valid")
	}
	if !IntentSecurity.Valid() {
		t.Fatalf("security must be valid")
	}
}

func TestParseIntent(t *testing.T) {
	if got := ParseIntent(" Coding "); got != IntentCoding {
		t.Fatalf("expected coding, got %q", got)
	}
	if got := ParseIntent("invalid"); got != IntentUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestRouterTaskIntent(t *testing.T) {
	if got := RouterTaskIntent("[router-task: coding]\nImplemente o plano e use a arquitetura."); got != IntentCoding {
		t.Fatalf("expected coding override, got %q", got)
	}
	if got := RouterTaskIntent("[router-task: invalid]\n[router-task: review]"); got != IntentReview {
		t.Fatalf("expected later valid override, got %q", got)
	}
	if got := RouterTaskIntent("[router-task: unknown]"); got != IntentUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestRouterAgentIntent(t *testing.T) {
	if got := RouterAgentIntent("Implementer"); got != IntentCoding {
		t.Fatalf("expected coding, got %q", got)
	}
	if got := RouterAgentTagIntent("[router-agent: reviewer]"); got != IntentReview {
		t.Fatalf("expected review, got %q", got)
	}
	if got := RouterAgentIntent("unknown"); got != IntentUnknown {
		t.Fatalf("expected unknown, got %q", got)
	}
}
