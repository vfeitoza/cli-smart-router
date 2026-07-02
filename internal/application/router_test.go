package application

import (
	"testing"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

func TestRouterOnlyHandlesConfiguredVirtualModel(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.VirtualModel = "smart:auto"
	cfg.Models = []domain.CandidateConfig{{Provider: "codex", Model: "gpt-5.4-mini"}}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{RequestedModel: "router:auto", AvailableProviders: []string{"codex"}})
	if got.Handled {
		t.Fatal("Route() handled the default model, want only configured virtual model")
	}

	got = router.Route(infrastructure.ModelRouteRequest{RequestedModel: "smart:auto", AvailableProviders: []string{"codex"}})
	if !got.Handled || got.TargetProvider != "codex" || got.TargetModel != "gpt-5.4-mini" {
		t.Fatalf("Route() = %#v, want codex/gpt-5.4-mini", got)
	}
}

func TestRouterSkipsUnavailableProviders(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Models = []domain.CandidateConfig{
		{Provider: "claude", Model: "claude-sonnet"},
		{Provider: "codex", Model: "gpt-5.4-mini"},
	}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{RequestedModel: domain.DefaultVirtualModel, AvailableProviders: []string{"codex"}})
	if !got.Handled || got.TargetProvider != "codex" || got.TargetModel != "gpt-5.4-mini" {
		t.Fatalf("Route() = %#v, want codex/gpt-5.4-mini", got)
	}
}

func TestRouterPrefersLowCostCandidate(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Models = []domain.CandidateConfig{
		{Provider: "claude", Model: "claude-opus", Cost: "very_high"},
		{Provider: "claude", Model: "claude-haiku", Cost: "low"},
	}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{RequestedModel: domain.DefaultVirtualModel, AvailableProviders: []string{"claude"}})
	if !got.Handled || got.TargetModel != "claude-haiku" {
		t.Fatalf("Route() = %#v, want low-cost claude-haiku", got)
	}
}

func TestRouterUsesStrongerModelForDetailedExplanation(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Models = []domain.CandidateConfig{
		{Provider: "claude", Model: "claude-opus", Cost: "very_high"},
		{Provider: "claude", Model: "claude-sonnet", Cost: "high"},
		{Provider: "claude", Model: "claude-haiku", Cost: "low"},
	}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{RequestedModel: domain.DefaultVirtualModel, AvailableProviders: []string{"claude"}, Body: []byte("explique-me detalhes sobre o frigate, como ele funciona")})
	if !got.Handled || got.TargetModel != "claude-sonnet" {
		t.Fatalf("Route() = %#v, want detailed prompt to use claude-sonnet", got)
	}
}
