# ADR 0005: Declarative Decision Engine Routing

## Status

Accepted

## Context

The existing `capability`, `benchmark`, `llm`, and `hybrid` strategies derive a
route from configured candidate metadata and optional classifier output. They
provide resilient defaults, but cannot guarantee policies such as routing every
security request to a particular model or routing large diffs to a designated
code model.

## Decision

Add the opt-in `decision_engine` strategy. It derives non-sensitive request
facts through local Prompt, Context, and Complexity analyzers, then evaluates
declarative `routes:` rules. The most specific matching valid rule wins; equal
specificity preserves declaration order. A rule target must be a configured,
available candidate.

When no rule matches, or the selected target is unavailable, routing falls back
the LLM classifier. Its ordered matching targets are retained only in bounded,

## Consequences

- Operators can define auditable routing policies by task, language,
  complexity, file count, diff presence, and streaming mode.
- `routes:` has no effect outside `strategy: decision_engine`, keeping existing
  strategy behavior unchanged.
- The management status endpoint exposes the last non-sensitive decision trace.
- Rules use conservative local heuristics and should be tuned from observed
  routes; unsupported or ambiguous requests remain protected by deterministic
  fallback.
