# ADR 0003: Local-First Hybrid Routing With Capability-Aware Confidence

## Status

Accepted

## Context

`strategy: hybrid` was implemented identically to `strategy: llm`: it always called the configured classifier through `host.model.execute` before falling back to deterministic routing. This meant every uncached, unpinned request paid an extra LLM round trip before the real request could execute, even when the answer was obvious from configuration alone.

At the same time, `models[].capabilities` was collected in configuration and forwarded to the classifier prompt, but the deterministic fallback path in `internal/domain/policy.go` ignored it entirely. Deterministic selection relied only on `cost`, `quality`, and a handful of hardcoded keyword checks (`detalhes`, `explique`, `código`, ...) to decide the minimum acceptable cost tier. `models[].capabilities` had no effect on which candidate was chosen locally.

This produced two related problems documented in `docs/analise.md`:

- `hybrid` was not actually faster or cheaper than `llm` in practice, defeating its purpose as a "balanced" strategy.
- The most useful piece of user-authored routing information (`capabilities`) was write-only: present in config, read by the classifier prompt, unused by the fallback that runs on every request and on every classifier failure.

## Decision

1. Add `domain.InferCapabilities(prompt string) []string`, a small keyword-based matcher that maps prompt text to capability tags (`coding`, `architecture`, `planning`, `summarize`, `translate`, `classify`, `tests`, `tools`, `agents`, `documentation`, `writing`, `review`, `refactor`, `reasoning`, `analysis`, `long_context`). It intentionally returns an empty list for ambiguous prompts instead of guessing.
2. Replace the single-candidate `SelectCandidateForPrompt` implementation with `domain.ScoreCandidates`, which:
   - filters candidates the same way as before (valid, available provider, minimum cost tier for the prompt);
   - gates that pool by inferred capabilities: if the prompt matched a known capability signal, only candidates that declare at least one of those capabilities remain eligible, unless no candidate declares any of them (in which case the gate is a no-op and behavior is unchanged from before this ADR);
   - ranks the remaining pool by the existing preference-driven tier logic (`quality` prefers highest quality tier; `cost`/`balanced` prefer lowest cost tier), so capability matching narrows the candidate pool without overriding cost/quality judgment.
3. Add `domain.RouteScore`, carrying `Candidate`, `Score`, `MatchedCount`, `InferredCount`, and `Reason`, plus `RouteScore.LocalConfident()`, which is true only when the prompt matched at least one known capability signal (`InferredCount > 0`) and the winning candidate satisfies at least one of them (`MatchedCount > 0`).
4. Add `domain.SelectCandidateWithConfidence`, returning the top `RouteScore` so callers can inspect confidence, not just the chosen candidate.
5. Add `domain.RouteDecision.Confident`, populated by `application.Router.Route` from `RouteScore.LocalConfident()`.
6. Change `strategy: hybrid` behavior in `cmd/plugin/main.go`:
   - run local deterministic scoring first (no host call);
   - if the local decision is `Handled` and `Confident`, use it directly, tagging the cache/debug reason with `local_confident`;
   - otherwise, call the classifier exactly as `llm` does, with deterministic fallback still available if the classifier fails.
7. Keep `strategy: llm` unchanged: it always calls the classifier first, regardless of local confidence. `llm` is for deployments that want a classifier opinion on every request; `hybrid` is for deployments that want classifier calls only when local signals are insufficient.
8. Extract `extractUserPrompt`/`contentToText` out of `cmd/plugin/main.go` into `infrastructure.ExtractUserPrompt`, and make `application.Router.Route` use it instead of the raw request body. Previously, the classifier and cache used the extracted last user message while deterministic fallback scored the full JSON body, which could produce different minimum-cost-tier judgments between the two paths for the same request.

## Rationale

- Capability tags already exist in configuration and are the most direct, auditable signal a deployment can give the router. Using them locally turns configuration into a real routing input instead of documentation-only metadata.
- A keyword matcher is intentionally simple: it does not need to be exhaustive. Any prompt it fails to recognize simply falls through to classifier-assisted routing (in `hybrid`) or the previous tier-only ranking (when no candidate declares matching capabilities), so there is no regression risk from an incomplete keyword list.
- Gating by capability instead of scoring by capability *count* avoids a subtle bias: a model declaring more tags is not automatically preferred over a more specific one. Preference (`cost`/`balanced`/`quality`) remains the sole ranking signal within the eligible pool, matching existing user expectations documented in ADR 0002.
- Requiring `MatchedCount > 0` *and* `InferredCount > 0` for confidence means the router never claims confidence it does not have: ambiguous prompts (no inferred capability) and configurations without capability tags (`InferredCount > 0` but no candidate ever matches) both correctly fall through to the classifier under `hybrid`.
- Unifying the prompt extraction removes a latent inconsistency where cache keys, classifier input, and deterministic scoring could reason about different text for the same request.

## Implementation Summary

- Added `internal/domain/capability.go` with `InferCapabilities` and a small, extensible `capabilitySignals` table.
- Rewrote `internal/domain/policy.go`: `ScoreCandidates`, `RouteScore`, `SelectCandidateWithConfidence`; `SelectCandidateForPrompt` and `SelectCandidate` are now thin wrappers over `ScoreCandidates` for backward compatibility.
- Added `RouteDecision.Confident` in `internal/domain/model.go`.
- Added `internal/infrastructure/prompt.go` with `ExtractUserPrompt`, used by both `application.Router.Route` and `cmd/plugin/main.go`.
- Updated `application.Router.Route` to extract the prompt once and propagate `Confident` and a capability-aware `Reason`.
- Updated `cmd/plugin/main.go`'s `routeModel` to branch strategy handling: `llm` keeps classifier-first behavior; `hybrid` is now local-first with classifier fallback only when not confident. Added `routeCacheEntryFromDecision` helper and a `router_local_confident` counter.
- Added unit tests for `InferCapabilities`, `ScoreCandidates`/`SelectCandidateWithConfidence` (summarize prompt, architecture prompt, quality preference, ambiguous prompt, no-capabilities-configured fallback), and `application.Router.Route` confidence behavior.

## Consequences

- `hybrid` requests with prompts matching configured capabilities skip the classifier entirely, removing one LLM round trip from the hot path for the common case.
- `hybrid` requests with ambiguous prompts, or configurations that omit `models[].capabilities`, behave exactly as before this ADR: classifier first, deterministic fallback on failure.
- Deployments that configure `models[].capabilities` now get real routing value from that field for both classifier context and local decisions.
- `llm` strategy is unchanged; deployments that want a classifier opinion on every request should keep using `llm`.
- Debug logs and cache reasons for hybrid local decisions are now tagged `local_confident`, making it possible to measure how often the classifier is skipped.

## Tradeoffs

- `InferCapabilities` is a keyword matcher, not a semantic classifier. It will miss capability signals expressed in unusual phrasing. This is an accepted tradeoff: missed signals fall through to the classifier (in `hybrid`) rather than causing an incorrect confident decision.
- The capability gate is binary (declares at least one matching tag or not); it does not weigh *how well* a candidate matches. This keeps the implementation simple and auditable, consistent with ADR 0002's preference for simple influencers over speculative weighted scoring.
- `strategy: llm` does not benefit from local-first behavior. Deployments currently on `llm` that want the latency/cost benefit must switch to `hybrid`.

## Rejected Alternatives

### Score By Capability Match Count

An earlier draft scored candidates additively, weighting capability match count above tier ranking (`score = matched*1000 + tierScore`). This was rejected because it let a model matching more tags outrank a cheaper/more appropriate model within the same task, contradicting the `balanced`/`cost` preference semantics from ADR 0002. The capability *gate* (filter, then rank by preference) was chosen instead.

### Numeric Confidence Threshold (e.g. `confidence >= 0.75`)

`docs/analise.md` proposed a numeric confidence threshold as one option. It was rejected in favor of the simpler boolean `LocalConfident()` rule (`InferredCount > 0 && MatchedCount > 0`) because it avoids introducing a new YAML knob and a magic threshold with no measured data to justify a specific value. If measured decision logs later show the boolean rule is too permissive or too conservative, a numeric threshold can be introduced as a follow-up without changing the public `RouteScore` shape.

### Also Changing `strategy: llm` To Be Local-First

Making `llm` local-first as well was considered, since it would give the same latency/cost benefit. It was rejected because `llm` is documented and used as the "always consult the classifier" strategy; deployments relying on that guarantee (for example, to gather classifier telemetry on every request) would silently lose it. `hybrid` remains the dedicated local-first strategy.

## Follow-Up Work

- Track `router_local_confident` versus classifier call counters in the management status endpoint to measure how often `hybrid` avoids the classifier in real deployments.
- Expand `capabilitySignals` based on observed ambiguous prompts in debug logs, without introducing a full NLP pipeline.
- Revisit a numeric confidence score only if debug-log evidence shows the boolean rule misclassifies confidence in practice.
- Consider applying the same capability gate to `applyPreferenceTiebreak` if classifier picks are observed crossing into capability-inappropriate models.
