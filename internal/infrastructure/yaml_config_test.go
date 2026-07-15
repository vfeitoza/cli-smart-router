package infrastructure

import "testing"

func TestParseConfigUsesConfiguredVirtualModel(t *testing.T) {
	raw := []byte(`{"config_yaml":"dmlydHVhbF9tb2RlbDogc21hcnQ6YXV0bwpzdHJhdGVneTogaHlicmlkCg=="}`)
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if cfg.VirtualModel != "smart:auto" {
		t.Fatalf("VirtualModel = %q, want smart:auto", cfg.VirtualModel)
	}
	if cfg.Strategy != "hybrid" {
		t.Fatalf("Strategy = %q, want hybrid", cfg.Strategy)
	}
}

func TestParseConfigParsesRoutesBlock(t *testing.T) {
	raw := []byte(`{"config_yaml":"cm91dGVzOgogIC0gd2hlbjoKICAgICAgdGFzazogY29kaW5nCiAgICAgIGxhbmd1YWdlOiBnbwogICAgICBjb21wbGV4aXR5OiBsb3cKICAgIHByb3ZpZGVyOiBjb2RleAogICAgbW9kZWw6IGdwdC01LjQtbWluaQogIC0gd2hlbjoKICAgICAgdGFzazogc2VjdXJpdHkKICAgIG1vZGVsOiBjbGF1ZGUtb3B1cy00LTgK"}`)
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("Routes len = %d, want 2", len(cfg.Routes))
	}
	first := cfg.Routes[0]
	if first.When.Task != "coding" || first.When.Language != "go" || first.When.Complexity != "low" {
		t.Fatalf("first rule when = %+v", first.When)
	}
	if first.Provider != "codex" || first.Model != "gpt-5.4-mini" {
		t.Fatalf("first rule target = %q/%q", first.Provider, first.Model)
	}
	if cfg.Routes[1].When.Task != "security" || cfg.Routes[1].Model != "claude-opus-4-8" {
		t.Fatalf("second rule = %+v", cfg.Routes[1])
	}
}
