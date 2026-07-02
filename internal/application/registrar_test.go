package application

import (
	"testing"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
)

func TestRegistrarUsesConfiguredVirtualModel(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.VirtualModel = "smart:auto"
	got := (Registrar{Config: cfg}).Register()
	if got.Provider != "smart-model-router" {
		t.Fatalf("Provider = %q, want smart-model-router", got.Provider)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "smart:auto" {
		t.Fatalf("Models = %#v, want smart:auto", got.Models)
	}
}
