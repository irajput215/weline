# Architecture decision records

An ADR records one hard-to-reverse decision: the context, what we chose, and what follows from it. Write one
whenever a design choice would be expensive to undo (SDLC phase 2, "Design").

- File: `wl-docs/adr/ADR-NNN-<short-title>.md`, numbered in order, never renumbered.
- Status moves `proposed` → `accepted`; a replaced decision becomes `superseded` (link the new ADR) rather
  than being edited or deleted.

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-001](ADR-001-modules-inside-weline.md) | Modules inside Weline under AGPL, joined by five seams | accepted |
| [ADR-002](ADR-002-connect-dont-build-pms.md) | Connect to each business's own system; don't build a practice management system | accepted |
| [ADR-003](ADR-003-scrub-patient-identifiers-from-logs.md) | Scrub patient identifiers from the process output | proposed (built, on by default) |
| [ADR-004](ADR-004-australian-hosting.md) | Host staging and production in Australia | proposed |
| [ADR-005](ADR-005-saas-model.md) | One shared Weline for all businesses; AGPL source link; Stripe; Meta bills businesses directly | proposed |

## Template

```markdown
# ADR-NNN: <decision>
Date: YYYY-MM-DD · Status: proposed | accepted | superseded

## Context
What forces the decision.

## Decision
What we chose.

## Consequences
What gets easier, what gets harder, how to undo it.
```
