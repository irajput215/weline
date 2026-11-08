# Phase 3: AI responses

**Goal:** clinic-aware automatic replies (booking intents and FAQ answers from the clinic's own knowledge) with a safe handover to a person.

**Prerequisites:** the [Phase 2 gate](phase-2-reminders.md#phase-2-gate) is passed.

[← Back to the guide](README.md)

## Steps

Run prompts A, B and C from [module-workflow.md](module-workflow.md) for the `ai` module.

Add to prompt B:

```text
Extra requirements for AI:
- kit.LLM interface so the provider can change; API key in config, never sent to the browser
- wl.ai_knowledge with pgvector embeddings (enable the extension in the migration), wl.ai_settings, wl.ai_runs
  (intent, confidence, latency, tokens, outcome; purge after 90 days)
- on message.incoming when no agent owns the chat and AI is on: classify intent (book, reschedule, cancel, FAQ,
  clinical, other). Booking intents call the booking service. FAQ answers only from the clinic's knowledge.
  Clinical, emergency or low-confidence messages hand over to a human through Weline's agent transfer.
- emergency keywords reply with "call 000" and hand over; never give clinical advice; label AI replies as automated
- POST /api/x/ai/reply also callable from a chatbot api_call node
- an eval set (testdata/ai_evals.jsonl) of de-identified clinic questions with expected intent and handover,
  and a command that runs it and fails CI below a threshold I choose
- prompts versioned in code
```

## Phase 3 gate

- [ ] The eval set passes
- [ ] Every clinical question goes to a person

Next: [Phase 4: Cliniko connector](phase-4-cliniko-connector.md).
