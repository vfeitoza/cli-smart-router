package application

import (
	"testing"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

func policyRouterConfig() domain.Config {
	return domain.Config{
		Enabled:      true,
		VirtualModel: "router:auto",
		Models: []domain.CandidateConfig{
			{Provider: "codex", Model: "gpt-5.4-mini", Cost: "low", Quality: "medium"},
			{Provider: "claude", Model: "claude-sonnet-5", Cost: "high", Quality: "high"},
			{Provider: "claude", Model: "claude-opus-4-8", Cost: "very_high", Quality: "highest"},
		},
	}
}

func TestPolicyRouterForwardsDecidedProvider(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	decision := domain.PolicyDecision{Matched: true, Provider: "codex", Model: "gpt-5.4-mini", Reason: "rule:task=coding"}
	got := r.Forward(decision, nil)
	if !got.Handled {
		t.Fatalf("expected handled")
	}
	if got.TargetProvider != "codex" || got.TargetModel != "gpt-5.4-mini" {
		t.Fatalf("unexpected target %s/%s", got.TargetProvider, got.TargetModel)
	}
}

func TestPolicyRouterUnmatchedDecisionNotHandled(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	got := r.Forward(domain.PolicyDecision{Matched: false}, nil)
	if got.Handled {
		t.Fatalf("expected not handled for unmatched decision")
	}
	if got.Reason != "policy_no_match" {
		t.Fatalf("unexpected reason %q", got.Reason)
	}
}

func TestPolicyRouterLocatesByModelWhenProviderOmitted(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	decision := domain.PolicyDecision{Matched: true, Model: "claude-sonnet-5"}
	got := r.Forward(decision, nil)
	if !got.Handled || got.TargetProvider != "claude" {
		t.Fatalf("expected claude provider, got %+v", got)
	}
}

func TestPolicyRouterProviderDisambiguation(t *testing.T) {
	cfg := policyRouterConfig()
	cfg.Models = append(cfg.Models, domain.CandidateConfig{Provider: "codex", Model: "claude-sonnet-5", Cost: "high", Quality: "high"})
	r := PolicyRouter{Config: cfg}
	decision := domain.PolicyDecision{Matched: true, Provider: "codex", Model: "claude-sonnet-5"}
	got := r.Forward(decision, nil)
	if got.TargetProvider != "codex" {
		t.Fatalf("expected codex via disambiguation, got %q", got.TargetProvider)
	}
}

func TestPolicyRouterRespectsAvailableProviders(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	decision := domain.PolicyDecision{Matched: true, Provider: "claude", Model: "claude-sonnet-5"}
	got := r.Forward(decision, []string{"codex"})
	if got.Handled {
		t.Fatalf("expected not handled when provider unavailable")
	}
	if got.Reason != "policy_provider_unavailable" {
		t.Fatalf("unexpected reason %q", got.Reason)
	}
}

func TestPolicyRouterUnknownModelNotHandled(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	got := r.Forward(domain.PolicyDecision{Matched: true, Model: "does-not-exist"}, nil)
	if got.Handled {
		t.Fatalf("expected not handled for unknown model")
	}
}

func TestPolicyRouterForwardRequestUsesRequestProviders(t *testing.T) {
	r := PolicyRouter{Config: policyRouterConfig()}
	decision := domain.PolicyDecision{Matched: true, Provider: "claude", Model: "claude-opus-4-8"}
	req := infrastructure.ModelRouteRequest{AvailableProviders: []string{"claude"}}
	got := r.ForwardRequest(decision, req)
	if !got.Handled || got.TargetModel != "claude-opus-4-8" {
		t.Fatalf("expected handled opus route, got %+v", got)
	}
}
