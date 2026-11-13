# AI agent: definition of done

Status: built (Phase 4, 2026-11-13). Items not done say why and who acts next.

- [x] R1–R12 implemented and tested:
  - Go: settings, knowledge, the pipeline (ignored, emergency, hand-over, knowledge answers, advice screen, booking,
    rescheduling, cancelling, labels, idempotency, purge), model clients for all three providers against fake servers,
    Weline's settings and key, handlers (permissions, `/reply`, isolation).
  - Vitest store tests, and two journeys.
- [ ] Open questions Q1–Q4 answered. **Not done:** built on the defaults (full-text search, "🤖" label, 90%/100%
  thresholds, the emergency word list). **You:** confirm or change them.
- [ ] Evals pass at the agreed threshold with a real model. **Not done:** needs an AI provider key. Run
  `WL_AI_EVAL=1 WL_AI_EVAL_PROVIDER=anthropic WL_AI_EVAL_MODEL=<model> WL_AI_EVAL_KEY=<key> go test -run TestEvals_RealModel -v ./internal/modules/ai/`
  (or ask me to run it once a key is added to the environment through the secure form).
- [x] Every clinical or emergency eval message hands over in the scripted evals (CI). Emergencies are caught by the
  word list even when the model calls them questions. With a real model: pending the run above.
- [ ] On staging, the agent books, reschedules and cancels in the Cliniko trial. **Not done:** needs the Cliniko trial,
  an AI key and a real phone.
- [x] No upstream file changed beyond the seams and the Weline accessor file (seam guard green; `HandOver` and
  `HasActiveTransfer` added to `internal/handlers/weline_hooks.go`, which is allowlisted).
- [x] Data model and threat model updated; release notes written.

## Notes

- With the agent on, turn off upstream's chatbot AI and fallback reply, or patients get two answers (the settings page
  says so). Upstream's keyword rules and flows still run.
- The agent replies only to messages patients just sent, so it's always inside the 24-hour window and never sends
  templates or marketing.
