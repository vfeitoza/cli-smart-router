package application

import (
	"encoding/json"
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

func chatBody(t *testing.T, userMessage string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": userMessage}},
	})
	if err != nil {
		t.Fatalf("marshal chat body: %v", err)
	}
	return body
}

func TestRouterIsConfidentForCapabilityMatchingPrompt(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Models = []domain.CandidateConfig{
		{Provider: "claude", Model: "claude-opus-4-8", Cost: "very_high", Quality: "highest", Capabilities: []string{"reasoning", "architecture", "planning"}},
		{Provider: "claude", Model: "claude-sonnet-5", Cost: "high", Quality: "high", Capabilities: []string{"coding", "reasoning", "architecture"}},
		{Provider: "claude", Model: "claude-haiku", Cost: "low", Quality: "medium", Capabilities: []string{"classify", "summarize"}},
	}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{
		RequestedModel:     domain.DefaultVirtualModel,
		AvailableProviders: []string{"claude"},
		Body:               chatBody(t, "Resuma este texto em 5 bullets"),
	})
	if !got.Handled || !got.Confident {
		t.Fatalf("Route() = %#v, want handled and confident for a clear summarize prompt", got)
	}
	if got.TargetModel != "claude-haiku" {
		t.Fatalf("Route() = %#v, want claude-haiku for a summarize prompt", got)
	}
}

func TestRouterIsNotConfidentForAmbiguousPrompt(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.Models = []domain.CandidateConfig{
		{Provider: "claude", Model: "claude-sonnet-5", Cost: "high", Quality: "high", Capabilities: []string{"coding", "reasoning", "architecture"}},
		{Provider: "claude", Model: "claude-haiku", Cost: "low", Quality: "medium", Capabilities: []string{"classify", "summarize"}},
	}
	router := Router{Config: cfg}

	got := router.Route(infrastructure.ModelRouteRequest{
		RequestedModel:     domain.DefaultVirtualModel,
		AvailableProviders: []string{"claude"},
		Body:               chatBody(t, "Me ajude a melhorar isso"),
	})
	if !got.Handled {
		t.Fatalf("Route() = %#v, want handled", got)
	}
	if got.Confident {
		t.Fatalf("Route() = %#v, want not confident for an ambiguous prompt", got)
	}
}
