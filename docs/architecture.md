# Smart Model Router Architecture

`smart-model-router` is a native CLIProxyAPI plugin that exposes one configurable virtual model and routes requests for that model to a configured built-in provider/model.

The implementation intentionally uses a minimal DDD-style layout. The goal is to keep routing policy testable and free from CLIProxyAPI ABI details, while keeping the native plugin entrypoint small and explicit.

## High-Level Flow

```text
Client request
  -> CLIProxyAPI
    -> model.route plugin call
      -> smart-model-router
        -> session route, if pinned
        -> cache route, if hit
        -> local capability-aware scoring (no host call)
        -> hybrid: use local decision if confident, else classifier
        -> llm: classifier first, always
        -> deterministic fallback (also the final answer for `capability`/`benchmark`)
      <- ModelRouteResponse(TargetKind: provider, Target, TargetModel)
    -> CLIProxyAPI executes selected provider/model
  <- Client response
```

By default, the plugin returns `TargetKind: provider`. CLIProxyAPI keeps responsibility for authentication, provider execution, retries, streaming, logging, and usage accounting.

The plugin only routes requests where `RequestedModel` matches the configured `virtual_model`. Every other model returns `Handled: false`.

## Package Layout

```text
cmd/plugin/
  main.go

internal/domain/
  config.go
  model.go
  policy.go
  capability.go

internal/application/
  router.go
  registrar.go
  usage.go

internal/infrastructure/
  cliproxy.go
  rpc.go
  yaml_config.go
  state.go
  runtime_state.go
  prompt.go
```

## `cmd/plugin`

`cmd/plugin/main.go` is the native plugin boundary.

Responsibilities:

- Export the C ABI expected by CLIProxyAPI.
- Store and call the host callback API.
- Dispatch plugin RPC methods.
- Load and normalize plugin configuration.
- Handle `model.route`, `model.register`, `usage.handle`, management routes, and optional executor fallback.
- Call host callbacks such as `host.model.execute` and `host.http.do`.
- Write non-sensitive debug route logs.

This package is allowed to know about:

- C ABI details
- CLIProxyAPI method names
- host callbacks
- JSON envelopes
- runtime state

It should not contain reusable domain policy unless that policy depends directly on host/runtime behavior.

## `internal/domain`

The domain package contains pure routing concepts and policy logic.

It must stay free of:

- C ABI details
- host callback details
- HTTP execution details
- CLIProxyAPI host implementation details

### `config.go`

Defines plugin configuration structs and normalization rules.

Important types:

- `Config`
- `DebugConfig`
- `CatalogConfig`
- `PricingConfig`
- `CacheConfig`
- `ExecutorFallbackConfig`
- `ClassifierConfig`
- `RoutingConfig`
- `CandidateConfig`

Important defaults:

- `DefaultVirtualModel = "router:auto"`
- `DefaultStrategy = "capability"`
- `DefaultPreference = "balanced"`

Supported preferences:

- `cost`
- `balanced`
- `quality`

### `model.go`

Defines normalized routing model concepts.

Important types:

- `Candidate`
- `RouteDecision`

`CandidateFromConfig` converts user configuration into normalized domain candidates.

### `policy.go`

Contains deterministic, capability-aware candidate scoring used by every strategy's fallback path (and by `hybrid`'s local-first decision).

Responsibilities:

- Filter invalid candidates.
- Respect available providers.
- Estimate minimum required cost tier from simple prompt signals.
- Gate eligible candidates by capabilities inferred from the prompt (see `capability.go`), when at least one configured candidate declares a matching capability.
- Apply `preference` to rank the gated pool.
- Apply conservative post-classifier preference tiebreaks.

Key types and functions:

- `RouteScore`: one scored candidate, with `MatchedCount`, `InferredCount`, `Reason`, and `LocalConfident()`.
- `ScoreCandidates`: scores and sorts every eligible candidate for a prompt.
- `SelectCandidateWithConfidence`: returns the top `RouteScore`.
- `SelectCandidateForPrompt` / `SelectCandidate`: thin wrappers returning only the chosen `Candidate`, kept for simpler call sites that do not need confidence.
- `ApplyPreferenceTiebreak`: unchanged conservative post-classifier tiebreak.

Cost tiers:

- `low`
- `medium`
- `high`
- `very_high`

Quality tiers:

- `low`
- `medium`
- `high`
- `highest`

Unknown cost or quality values rank as `medium`.

### `capability.go`

Contains `InferCapabilities(prompt string) []string`, a small keyword-based matcher that maps prompt text to capability tags such as `coding`, `architecture`, `planning`, `summarize`, `translate`, `classify`, `tests`, `tools`, `agents`, `documentation`, `writing`, `review`, `refactor`, `reasoning`, `analysis`, and `long_context`.

It intentionally returns an empty slice for ambiguous prompts. Callers must treat an empty result as "no local capability signal", not as "no capabilities apply".

## `internal/application`

The application package contains use cases that coordinate domain logic and infrastructure contract types.

### `router.go`

The `Router` use case handles deterministic, capability-aware route selection.

It:

- Normalizes config.
- Ignores requests that do not match `virtual_model`.
- Converts configured models into domain candidates.
- Extracts the last user message via `infrastructure.ExtractUserPrompt` (the same prompt used by cache keys and the classifier).
- Calls `domain.SelectCandidateWithConfidence`.
- Returns a `RouteDecision`, including `Confident` for callers implementing local-first strategies.

Classifier routing is not implemented here because classifier execution requires host callbacks. That host-aware behavior lives in `cmd/plugin/main.go`.

### `registrar.go`

Registers the virtual model with CLIProxyAPI.

The registered model is configurable through `virtual_model`.

### `usage.go`

Aggregates usage signals without storing sensitive request/response content.

## `internal/infrastructure`

The infrastructure package contains local contracts and state holders.

### `cliproxy.go`

Defines the minimal CLIProxyAPI JSON/ABI contract structs used by this plugin.

The project currently uses local contract structs instead of importing the CLIProxyAPI SDK because this workspace uses Go 1.22 and the local CLIProxyAPI SDK requires Go 1.26.

The SDK import decision should be revisited only when the project toolchain is Go 1.26 or newer.

### `rpc.go`

Builds and parses plugin RPC envelopes.

### `yaml_config.go`

Parses plugin YAML configuration from CLIProxyAPI plugin config payloads.

### `state.go`

Stores the current normalized configuration in memory.

### `runtime_state.go`

Stores non-sensitive runtime state.

Runtime state may include:

- catalog model IDs
- pricing fetch metadata
- usage aggregates
- session route pins
- route cache entries
- counters, including `router_local_confident` for hybrid local-first decisions

Runtime state must not include:

- prompts
- request bodies
- response bodies
- credentials
- API keys
- auth records

### `prompt.go`

Extracts the last user message from OpenAI/Claude `messages` or Gemini `contents.parts` request bodies via `ExtractUserPrompt`. This is the single prompt-extraction implementation shared by cache keys, classifier input, and deterministic/local-first candidate scoring, so all three reason about the same text for a given request.

## Routing Pipeline

For a matching `virtual_model`, route selection happens in this order.

### 1. Session Route

If `routing.keep_same_model_per_session` is enabled and request metadata contains `execution_session_id`, the plugin checks whether that session is already pinned to a route.

If found, the route is reused and logged with `source: session`.

### 2. Route Cache

If `cache.enabled` is true, the plugin checks the route cache.

The cache key is based on:

- configured virtual model
- extracted last user message

The raw prompt is not persisted. Only the hash is stored as a map key.

Cache behavior:

- LRU eviction when `cache.max_entries` is reached.
- Optional TTL expiration with `cache.ttl`.
- State saves are throttled to avoid rewriting the state file on every request.

If found, the route is reused and logged with `source: cache`.

### 3. Local Capability-Aware Scoring

Every strategy computes a local, deterministic decision through `application.Router.Route`, which never makes a host call. This is the same decision used as the final answer for `capability`/`benchmark` and as the deterministic fallback for `llm`/`hybrid` on classifier failure.

Local scoring:

- extracts capabilities implied by the prompt via `domain.InferCapabilities` (a small keyword matcher, see `capability.go`);
- filters eligible candidates by validity, available providers, and minimum cost tier for the prompt (same as before);
- gates that pool to candidates declaring at least one inferred capability, when at least one configured candidate does so; otherwise the gate is a no-op;
- ranks the gated pool by `preference` (unchanged tier logic).

The result includes whether the decision is "locally confident": true only when the prompt matched a known capability signal and the winning candidate satisfies it. An ambiguous prompt, or a configuration with no matching `capabilities`, is never locally confident even though a candidate is still deterministically chosen.

### 4. Strategy-Specific Use Of The Local Decision And Classifier

- `capability` / `benchmark`: use the local decision directly. No classifier call.
- `llm`: always calls the classifier first (see below), regardless of local confidence. Deterministic fallback (the same local decision) is used only if the classifier fails.
- `hybrid`: local-first. If the local decision is confident, it is used directly and tagged `local_confident` in cache/debug reasons, skipping the classifier entirely. If not confident, the classifier is called exactly as in `llm`, with deterministic fallback on classifier failure.

### 5. Classifier Route

Classifier routing is attempted when:

- `strategy` is `llm`, always; or `strategy` is `hybrid` and the local decision above was not confident
- `classifier.enabled` is true
- at least one classifier model is configured

Classifier models are tried in order up to `classifier.max_attempts`.

The classifier call uses `host.model.execute` and sends an isolated routing prompt. It receives:

- the configured candidate catalog
- provider/model IDs
- cost tiers
- quality tiers
- capabilities
- the extracted last user message
- the configured `preference` instruction

The classifier must return compact JSON with:

```json
{"selected_model":"<id>","confidence":0.9,"reason":"short reason"}
```

Classifier failure cases:

- host call error
- non-2xx model execution response
- invalid JSON
- empty `selected_model`
- selected model not found in configured `models`
- selected model provider unavailable

On failure, the next classifier is tried. If all attempts fail, the local deterministic decision computed in step 3 is used.

### 6. Preference Tiebreak

After a classifier selects a model, the plugin may apply a conservative preference tiebreak.

Behavior:

- `balanced`: no post-classifier tiebreak.
- `quality`: among candidates in the same cost tier as the classifier pick, prefer the highest quality.
- `cost`: among candidates in the same quality tier as the classifier pick, prefer the cheapest.

This is intentionally conservative. It does not override the classifier's complexity judgment by jumping across cost/quality tiers.

When a tiebreak changes the model, the route reason includes:

```text
preference_tiebreak:old-model->new-model
```

## Strategy Semantics

Supported strategy values:

- `capability`
- `benchmark`
- `llm`
- `hybrid`

Current implementation behavior:

- `capability`: local capability-aware deterministic decision only. No classifier call.
- `benchmark`: currently behaves like `capability`; benchmark fields are reserved metadata.
- `llm`: classifier first on every request, local deterministic decision as fallback on classifier failure.
- `hybrid`: local-first. Uses the local decision directly when it is confident (skipping the classifier); otherwise behaves like `llm` for that request.

The plugin metadata exposes all four values, and the local deterministic decision remains the safety path for every strategy.

## Preference Semantics

Supported preference values:

- `cost`
- `balanced`
- `quality`

Default: `balanced`

Invalid values normalize to `balanced`.

Preference affects:

- classifier prompt bias
- deterministic fallback ranking
- conservative post-classifier tiebreaks

Preference does not force every request to one extreme. A simple prompt should not be promoted to the most expensive model just because `preference: quality` is set.

## Provider Model Routing

The normal route response uses provider routing:

```json
{
  "Handled": true,
  "TargetKind": "provider",
  "Target": "claude",
  "TargetModel": "claude-sonnet-5",
  "Reason": "classifier:..."
}
```

Provider routing lets CLIProxyAPI keep its normal execution path.

The example config uses built-in provider keys:

- `claude`
- `codex`

## Optional Executor Fallback

When `executor_fallback.enabled` is true, the plugin declares executor capability.

For non-streaming requests, route responses may target the plugin itself:

```json
{
  "Handled": true,
  "TargetKind": "self",
  "Reason": "... executor_fallback"
}
```

The executor fallback then tries configured candidates through `host.model.execute`.

Limits:

- non-streaming only
- streaming executor fallback is not implemented
- disabled by default

## External State Refresh

The plugin may refresh external state before routing.

### Catalog Refresh

When `catalog.base_url` is configured, the plugin fetches:

```text
GET <base_url>/v1/models
```

through `host.http.do`.

The result is used as non-sensitive catalog state only. Configured candidate metadata remains authoritative.

### Pricing Refresh

When `pricing.enabled` and `pricing.url` are configured, the plugin fetches pricing metadata through `host.http.do`.

Routing must continue if this fetch fails.

The runtime state stores metadata such as byte count and error text, not sensitive payloads.

## Debug Logging

When `debug.enabled` and `debug.log_path` are configured, the plugin writes one JSON object per routed request.

Example fields:

- `time`
- `source`
- `virtual_model`
- `source_format`
- `stream`
- `strategy`
- `preference`
- `target_provider`
- `target_model`
- `reason`
- `classifier`

Possible `source` values:

- `selected`
- `cache`
- `session`

When `strategy: hybrid` uses the local confident decision instead of calling the classifier, `source` is still `selected`, and `reason` is prefixed with `local_confident` so decision logs distinguish it from a classifier or plain deterministic-fallback selection.

Classifier trace may include:

- whether classifier was enabled
- whether classifier was used
- classifier model used
- raw classifier response
- classifier error

Debug logs intentionally omit prompts and request/response bodies.

## Management API

The plugin registers a management route:

```text
GET /plugins/smart-model-router/status
```

The status response includes:

- plugin identifier
- virtual model
- strategy
- usage snapshot
- runtime state snapshot

## Security Boundaries

The plugin must not log or persist:

- prompts
- request bodies
- response bodies
- credentials
- API keys
- auth records

Debug logs and state are intentionally non-sensitive.

Classifier routing receives the extracted last user message in memory because it must classify the request. That text is not written to the plugin state or debug logs.

## Design Constraints

- Keep domain code independent from ABI and host callback details.
- Keep configured model metadata authoritative.
- Keep deterministic fallback available for every advanced feature.
- Avoid interfaces with one implementation unless they protect domain/application code from host or ABI details.
- Do not import the CLIProxyAPI SDK until the project toolchain can use Go 1.26 or newer.
