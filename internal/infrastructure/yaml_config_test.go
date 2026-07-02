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
