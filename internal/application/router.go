package application

import (
	"fmt"
	"strings"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

// Router handles the model routing use case.
type Router struct {
	Config domain.Config
}

// Route chooses a provider/model for the configured virtual model.
func (r Router) Route(req infrastructure.ModelRouteRequest) domain.RouteDecision {
	cfg := r.Config.Normalize()
	if !cfg.Enabled || strings.TrimSpace(req.RequestedModel) != cfg.VirtualModel {
		return domain.RouteDecision{Handled: false}
	}
	candidates := make([]domain.Candidate, 0, len(cfg.Models))
	for _, item := range cfg.Models {
		candidates = append(candidates, domain.CandidateFromConfig(item))
	}
	candidate, ok := domain.SelectCandidateForPrompt(candidates, req.AvailableProviders, string(req.Body), cfg.Preference)
	if !ok {
		return domain.RouteDecision{Handled: false, Reason: "no_available_candidate"}
	}
	return domain.RouteDecision{
		Handled:        true,
		TargetProvider: candidate.Provider,
		TargetModel:    candidate.Model,
		Reason:         fmt.Sprintf("deterministic_fallback strategy:%s preference:%s provider:%s cost:%s", cfg.Strategy, cfg.Preference, candidate.Provider, candidate.Cost),
	}
}
