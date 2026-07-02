# Smart Model Router

`smart-model-router` is a native CLIProxyAPI plugin that registers a configurable virtual model and routes requests for that model to a real built-in provider/model.

The default virtual model is `router:auto`, but the name is configurable through `virtual_model`.

## Current Scope

This repository currently implements the router core plus the advanced features from the implementation plan:

- Native CLIProxyAPI C ABI entrypoint.
- `plugin.register` / `plugin.reconfigure` lifecycle.
- `model.register` virtual model registration.
- `model.route` routing for the configured virtual model only.
- Provider filtering through `AvailableProviders`.
- `/v1/models` catalog refresh through `host.http.do` when configured.
- External pricing fetch metadata through `host.http.do` when configured.
- Deterministic route cache with last-user-message hash keys, LRU eviction, and optional TTL.
- Session affinity for stable per-session routing.
- Optional LLM classifier routing through `host.model.execute` with isolated routing prompts.
- Cost/balanced/quality routing preference through `preference`.
- Optional same-request non-streaming fallback through plugin executor mode.
- `usage.handle` counters and persisted non-sensitive runtime state.
- Authenticated Management API status route.
- DDD-inspired package layout.

## Architecture

The code follows a small DDD-inspired structure:

```text
cmd/plugin/
  main.go                  # Native plugin ABI and method dispatch
internal/domain/
  config.go                # Config entities and normalization
  model.go                 # Candidate and decision types
  policy.go                # Deterministic candidate selection
internal/application/
  router.go                # Routing use case
  registrar.go             # Virtual model registration use case
  usage.go                 # Usage counters
internal/infrastructure/
  cliproxy.go              # Minimal CLIProxyAPI JSON contracts used by V1
  rpc.go                   # RPC envelope helpers
  yaml_config.go           # config_yaml parsing
  state.go                 # In-memory config store
  runtime_state.go         # Non-sensitive runtime state, cache, catalog, pricing metadata
```

All code comments and configuration field descriptions are in English.

## Routing Behavior

The plugin only handles requests where `RequestedModel` matches `virtual_model`.

For all other models, it returns:

```json
{"Handled": false}
```

For the configured virtual model, it chooses a configured candidate whose provider is currently present in `AvailableProviders`. Configured `models` remain the authoritative source for provider, capability, cost, and quality metadata.

Decision order:

1. Existing session route, when session affinity is enabled.
2. Existing cache route, when cache is enabled and a valid entry exists.
3. Classifier route, when `strategy` is `llm` or `hybrid`.
4. Deterministic fallback.

The classifier receives an isolated routing prompt with the configured model catalog and the extracted last user message. It must select a configured model; invalid classifier output falls back to deterministic routing.

Default provider-route response:

```json
{
  "Handled": true,
  "TargetKind": "provider",
  "Target": "codex",
  "TargetModel": "gpt-5.4-mini",
  "Reason": "classifier:short routing reason"
}
```

When `executor_fallback.enabled: true` and the request is non-streaming, `model.route` returns `TargetKind: "self"`. The plugin executor then tries configured candidates in order with `host.model.execute` until one succeeds.

## Configuration Example

Ready-to-use example:

```text
configs/smart-model-router.yaml
```

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    smart-model-router:
      enabled: true
      priority: 100

      virtual_model: router:auto
      strategy: hybrid
      preference: balanced
      state_path: smart-model-router-state.json

      debug:
        enabled: true
        log_path: smart-model-router-decisions.jsonl

      catalog:
        source: cli_proxy_api
        base_url: http://127.0.0.1:8317
        api_key: ""
        refresh_interval: 10m
        include_router_model: false

      pricing:
        enabled: true
        url: https://raw.githubusercontent.com/ENTERPILOT/ai-model-price-list/refs/heads/main/sources/llm_prices_current.json
        refresh_interval: 6h

      cache:
        enabled: true
        max_entries: 1024
        ttl: 24h

      executor_fallback:
        enabled: false
        max_attempts: 3

      classifier:
        enabled: true
        models:
          - provider: codex
            model: gpt-5.4-mini
          - provider: claude
            model: claude-haiku-4-5-20251001
        timeout: 8s
        max_attempts: 2

      routing:
        prefer_low_cost: true
        prefer_low_latency: false
        prefer_high_quality: true
        max_cost_per_request: 0.05
        max_input_tokens: 100000
        keep_same_model_per_session: true
        allow_fallback: true
        switch_threshold: 0.15
        benchmark_weight: 0.4
        llm_router_weight: 0.3
        capability_weight: 0.3

      models:
        - provider: codex
          model: gpt-5.4-mini
          capabilities: [classify, summarize, translate, simple_coding, fast, low_cost, routing]
          cost: low
          quality: medium
        - provider: claude
          model: claude-sonnet-5
          capabilities: [reasoning, writing, coding, architecture]
          cost: high
          quality: high
```

## Configuration Fields

| Field | Type | Description |
| --- | --- | --- |
| `virtual_model` | string | Virtual model name intercepted by the router. Defaults to `router:auto`. |
| `strategy` | enum | Routing strategy: `capability`, `benchmark`, `llm`, or `hybrid`. `llm`/`hybrid` try classifier routing before deterministic fallback. |
| `preference` | enum | Decision influencer: `cost`, `balanced`, or `quality`. Defaults to `balanced`. |
| `state_path` | string | Optional local JSON file for non-sensitive runtime state. |
| `debug` | object | Optional JSONL route decision logging. |
| `catalog` | object | `/v1/models` catalog refresh settings. |
| `pricing` | object | External pricing refresh settings. |
| `cache` | object | Deterministic route cache settings: enabled flag, `max_entries`, and optional `ttl`. |
| `executor_fallback` | object | Optional non-streaming same-request fallback executor settings. |
| `classifier` | object | Ordered classifier routing settings. Classifier models are tried in order and must select configured candidates. |
| `routing` | object | Policy weights and limits. |
| `models` | array | Candidate provider/model matrix. |

Complete field documentation is available in `docs/configuration.md`.

## Strategy And Preference

Supported strategies:

- `capability`: deterministic fallback only.
- `benchmark`: currently behaves like deterministic fallback; benchmark scoring is reserved for future measured data.
- `llm`: classifier first, deterministic fallback on failure.
- `hybrid`: same operational behavior as `llm` today.

Supported preferences:

- `cost`: bias toward the cheapest acceptable model.
- `balanced`: default balance of cost and quality.
- `quality`: bias toward higher-quality models.

`preference` biases the classifier prompt, affects deterministic fallback ranking, and applies conservative post-classifier tiebreaks among equivalent-tier models.

## Deterministic Cache

When `cache.enabled` is true, repeated identical prompts reuse the same route decision.

Cache behavior:

- Keyed by the configured virtual model and extracted last user message.
- Raw prompts are not stored; only a hash key is persisted.
- `cache.max_entries` bounds cache size.
- LRU eviction removes the least recently used entry when full.
- `cache.ttl` optionally expires old decisions.

The first request logs `source: selected`; repeated matching prompts should log `source: cache`.

## Management Status

The plugin registers this management route:

```text
GET /v0/management/plugins/smart-model-router/status
```

The route is exposed by CLIProxyAPI under `/v0/management/...` and requires the management key.

The response includes runtime state snapshots for catalog, pricing, counters, cache, sessions, and usage. It never stores prompts, request bodies, credentials, API keys, or response bodies.

## Debug Route Logs

Enable non-sensitive JSONL route decision logs:

```yaml
debug:
  enabled: true
  log_path: smart-model-router-decisions.jsonl
```

Each line includes the selected provider/model, strategy, preference, source, reason, and classifier trace when applicable. It does not log prompts, request bodies, credentials, API keys, or responses.

## Documentation

- `docs/configuration.md`: complete configuration reference.
- `docs/architecture.md`: package layout and routing pipeline.
- `docs/business-rules.md`: product/business routing rules.
- `docs/adr/`: architecture decision records.

## Development

Run tests:

```bash
go test ./...
```

Build the Linux shared library:

```bash
make build-local
```

See `INSTALL.md` for installation details.
