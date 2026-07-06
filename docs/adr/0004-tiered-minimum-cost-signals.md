# ADR 0004: Tiered Minimum Cost Signals

## Status

Accepted

## Context

The deterministic fallback path estimates a minimum acceptable cost tier before capability gating and preference ranking. Previously this used a very small set of hardcoded signals: simple arithmetic/classification/summary mapped to `low`, while details/explanation/code prompts mapped to `high`.

That was too coarse for local-first routing. It could under-route planning, architecture, migration, production, or security prompts to cheaper tiers, and it did not use the existing `medium` tier for smaller but non-trivial work such as scripts or documentation.

## Decision

Replace the tiny two-level signal check with conservative keyword groups by cost tier:

- `very_high`: architecture, system design, migration, monorepo, production, security, or critical work.
- `high`: planning, strategy, explanation, analysis, coding, implementation, refactor, debugging, tests, and reviews.
- `medium`: scripts, SQL, regex, documentation, formatting, and rewrites.
- `low`: simple arithmetic, classification, summary, and translation.

The implementation remains local and deterministic in `internal/domain/policy.go`. It intentionally does not introduce configurable rule files, weights, regex engines, or a new policy package.

## Consequences

- Deterministic fallback and confident `hybrid` local decisions escalate important planning and architecture prompts before preference ranking.
- `medium` cost candidates can be selected for non-trivial but bounded utility tasks.
- `llm` strategy behavior is unchanged when classification succeeds, but its fallback is safer when the classifier fails.
- Keyword matching remains approximate. If real route traces show repeated misrouting, revisit with the smallest useful change before adding a configurable rules engine.
