# Phase 4: AI agent

**Goal:** a WhatsApp agent that answers from the business's own information, books, reschedules and cancels through
the connector, and hands anything clinical, urgent or uncertain to a person.

**Prerequisites:** the [Phase 3 gate](phase-3-connectors-cliniko.md#phase-3-gate) is passed.

[← Back to the guide](README.md)

## Steps

Run prompts A, B and C from [module-workflow.md](module-workflow.md) for the `ai` module.

Add to prompt A: "Weline already has AI contexts (chatbot → AI) with OpenAI, Anthropic and Google providers, static or
API context and keyword triggers. Say in design.md what we reuse (provider settings, keys) and what the module adds."

Add to prompt B:

```text
Extra requirements for AI:
- kit.LLM interface so the provider can change; reuse Weline's AI provider settings and keys; keys never sent to
  the browser.
- Knowledge base per organization: upload documents, FAQs and website URLs; chunk and embed into wl.ai_knowledge with
  pgvector (enable the extension in the migration); answers only from this knowledge, with the source kept in
  wl.ai_runs.
- wl.ai_settings (on/off, hours, handover team, tone, vertical pack instructions) and wl.ai_runs (intent, confidence,
  latency, tokens, outcome; purge after 90 days).
- On message.incoming when no agent owns the chat and AI is on: classify intent (book, reschedule, cancel, question,
  clinical, other). Booking intents use the connector abilities (FindFreeTimes, Book, Reschedule, Cancel) for the
  contact in the chat. Questions are answered only from the knowledge base.
- Clinical, emergency, complaint or low-confidence messages hand over to a person through Weline's agent transfer.
  Emergency keywords reply with "call 000" and hand over. Never give clinical advice. Label AI replies as automated.
- Respect consent (Phase 2) and the 24-hour window: outside it, only templates.
- POST /api/x/ai/reply also callable from a chatbot api_call node.
- An eval set (testdata/ai_evals.jsonl) of de-identified clinic questions with the expected intent, answer source
  and handover, and a command that runs it and fails CI below a threshold I choose.
- Prompts versioned in code.
```

## Phase 4 gate

- [ ] The eval set passes at the agreed threshold
- [ ] Every clinical or emergency question in the set goes to a person
- [ ] On staging, the agent books, reschedules and cancels in the Cliniko trial correctly

Next: [Phase 5: Reminders and recalls](phase-5-reminders-recalls.md).
