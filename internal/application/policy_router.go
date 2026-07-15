package application

import (
	"fmt"
	"strings"

	"github.com/vfeitoza/cli-smart-router/internal/domain"
	"github.com/vfeitoza/cli-smart-router/internal/infrastructure"
)

// PolicyRouter is the Model Router layer. It is intentionally decision-free: it
// only receives a PolicyDecision (from the Policy Engine), locates the matching
// provider among the configured candidates and available providers, and produces
// the forwarding target. It never scores, ranks, or picks a model on its own.
type PolicyRouter struct {
	Config domain.Config
}

// Forward turns a Policy Engine decision into a route target. It performs no
// model selection: if the decision did not match a rule, or the decided
// provider/model cannot be located among configured candidates and available
// providers, it returns Handled:false so the caller falls back to the existing
// deterministic routing. When located, it returns the provider route verbatim.
func (r PolicyRouter) Forward(decision domain.PolicyDecision, availableProviders []string) domain.RouteDecision {
	if !decision.Matched {
		return domain.RouteDecision{Handled: false, Reason: "policy_no_match"}
	}
	candidate, ok := r.locateProvider(decision, availableProviders)
	if !ok {
		return domain.RouteDecision{Handled: false, Reason: "policy_provider_unavailable"}
	}
	return domain.RouteDecision{
		Handled:        true,
		TargetProvider: candidate.Provider,
		TargetModel:    candidate.Model,
		Reason:         fmt.Sprintf("policy_engine %s provider:%s", decision.Reason, candidate.Provider),
		Confident:      true,
	}
}

// locateProvider finds the configured candidate the decision points to and
// verifies its provider is available. When the decision names a provider, both
// provider and model must match; otherwise the first candidate with the model id
// is used. This is pure location — no selection heuristics.
func (r PolicyRouter) locateProvider(decision domain.PolicyDecision, availableProviders []string) (domain.Candidate, bool) {
	cfg := r.Config.Normalize()
	wantModel := strings.TrimSpace(decision.Model)
	wantProvider := strings.ToLower(strings.TrimSpace(decision.Provider))
	if wantModel == "" {
		return domain.Candidate{}, false
	}

	available := domain.AvailableSet(availableProviders)

	for _, candidate := range cfg.Candidates() {
		if !candidate.Valid() || candidate.Model != wantModel {
			continue
		}
		if wantProvider != "" && candidate.Provider != wantProvider {
			continue
		}
		if len(available) > 0 {
			if _, ok := available[candidate.Provider]; !ok {
				continue
			}
		}
		return candidate, true
	}
	return domain.Candidate{}, false
}

// ForwardRequest is a convenience wrapper for the model.route path: it forwards a
// PolicyDecision using the providers advertised on the incoming request.
func (r PolicyRouter) ForwardRequest(decision domain.PolicyDecision, req infrastructure.ModelRouteRequest) domain.RouteDecision {
	return r.Forward(decision, req.AvailableProviders)
}
