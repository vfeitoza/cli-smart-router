package domain

func routeTestCandidates() []Candidate {
	return []Candidate{
		{Provider: "codex", Model: "gpt-5.4-mini", Cost: "low", Quality: "medium"},
		{Provider: "claude", Model: "claude-sonnet-5", Cost: "high", Quality: "high"},
		{Provider: "claude", Model: "claude-opus-4-8", Cost: "very_high", Quality: "highest"},
		{Provider: "codex", Model: "kimi", Cost: "low", Quality: "medium"},
	}
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }
