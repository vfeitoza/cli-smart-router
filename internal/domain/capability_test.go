package domain

import "testing"

func TestInferCapabilitiesSummarizeSignal(t *testing.T) {
	got := InferCapabilities("Resuma este texto em 5 bullets")
	if !containsString(got, "summarize") {
		t.Fatalf("InferCapabilities() = %v, want summarize", got)
	}
}

func TestInferCapabilitiesArchitectureSignal(t *testing.T) {
	got := InferCapabilities("Planeje a arquitetura de um sistema multi-tenant com filas e retries")
	for _, want := range []string{"architecture", "planning"} {
		if !containsString(got, want) {
			t.Fatalf("InferCapabilities() = %v, want %s", got, want)
		}
	}
}

func TestInferCapabilitiesCodingErrorSignal(t *testing.T) {
	got := InferCapabilities("corrija este erro Go: panic: nil pointer dereference")
	if !containsString(got, "coding") {
		t.Fatalf("InferCapabilities() = %v, want coding", got)
	}
}

func TestInferCapabilitiesAmbiguousPromptReturnsEmpty(t *testing.T) {
	got := InferCapabilities("Me ajude a melhorar isso")
	if len(got) != 0 {
		t.Fatalf("InferCapabilities() = %v, want empty for ambiguous prompt", got)
	}
}

func TestInferCapabilitiesLongPromptAddsLongContext(t *testing.T) {
	long := make([]byte, longPromptThreshold+1)
	for i := range long {
		long[i] = 'a'
	}
	got := InferCapabilities(string(long))
	if !containsString(got, "long_context") {
		t.Fatalf("InferCapabilities() = %v, want long_context for long prompt", got)
	}
}
