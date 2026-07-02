# ADR 0001: Implement Advanced Routing Features Incrementally

## Status

Superseded

## Context

`smart-model-router` must work as a native CLIProxyAPI model router. The safest host contract is `model.route` returning `TargetKind: "provider"`, which keeps execution, auth selection, logging, and usage accounting inside CLIProxyAPI.

The original product idea includes live catalog discovery, LLM-based classification, benchmark learning, pricing, semantic cache, and same-request fallback. These features are useful, but they add host callback complexity, state management, latency, and failure modes.

## Decision

Initial V1 implemented only the deterministic core:

- configurable virtual model registration;
- intercept only `virtual_model`;
- route to the first configured available provider/model;
- minimal in-memory usage counters;
- management status endpoint.

The following features were deferred initially and are now implemented incrementally:

- live `/v1/models` catalog fetch through `host.http.do`;
- LLM classifier fallback through `host.model.execute`;
- persistent non-sensitive runtime state;
- external pricing fetch metadata;
- same-request non-streaming execution fallback through a plugin executor.

Semantic similarity cache and quality judge remain outside the current scope.

## Rationale

- `model.route` should stay fast and reliable.
- Returning `TargetKind: "provider"` cannot retry another model after an upstream execution failure in the same request.
- Same-request fallback requires `TargetKind: "self"` plus executor orchestration, which is a larger and riskier capability.
- `/v1/models` does not guarantee provider/capability metadata, so configured candidates remain the safe source of routing metadata.
- LLM classification adds cost and latency; it must never be required for the router to function.
- Persistent state and pricing need careful no-secret/no-prompt storage rules.

## Consequences

- The router still works when catalog, pricing, or classifier calls fail.
- Configured `models` remain authoritative for provider and capability metadata.
- Executor fallback is opt-in and non-streaming only.
- Future ADRs should document semantic cache or judge-based quality scoring if added.
