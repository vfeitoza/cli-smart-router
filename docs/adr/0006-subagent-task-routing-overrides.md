# ADR 0006: Subagent Task Routing Overrides

## Status

Accepted

## Context

OpenCode creates a new model request for each delegated subagent, but an
implementer prompt can include a plan, roadmap, or architecture specification.
The local keyword analyzer then correctly sees those words but incorrectly
routes the implementation request as planning.

## Decision

Allow trusted clients to declare the phase through `X-Router-Task`. If it is
missing or invalid, use a known `X-Router-Agent`, then equivalent tags in the
last user message, before the keyword analyzer. Supported agent mappings are
`planner` to `planning`, `implementer` to `coding`, and `reviewer` to `review`.

Include the effective override in the route-cache key.

## Consequences

- OpenCode subagents retain an appropriate route across tool-call turns.
- Existing clients without headers or tags preserve heuristic behavior.
- The declaration is a client-provided routing signal, not authorization; the
  plugin still validates the resulting rule target against configured and
  available providers.
