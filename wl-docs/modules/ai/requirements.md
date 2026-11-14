# AI agent: requirements

**For:** the business owner or manager (clinic: practice manager) who wants WhatsApp questions answered and bookings
made out of hours, and reception, who take over anything the agent shouldn't handle. Build guide Phase 4
([`../../build-guide/phase-4-ai-agent.md`](../../build-guide/phase-4-ai-agent.md)).

Status: **built on the defaults below; open questions still to confirm.**

## User stories

- As a patient, I ask a question on WhatsApp (opening hours, parking, prices, what to bring) and get a correct answer
  from the clinic's own information, marked as automated.
- As a patient, I book, move or cancel an appointment in the chat, in the clinic's system (built-in or Cliniko).
- As a patient with a clinical question or an emergency, I'm never given advice: I'm told to call 000 in an
  emergency, and a person takes over.
- As a practice manager, I turn the agent on, give it our information (FAQs and documents), choose the team that gets
  handovers, and see what it did and why.

## Functional requirements

- R1 **Uses Weline's AI provider settings and key** (Settings → Chatbot → AI: OpenAI, Anthropic or Google, model, key).
  The key is decrypted on the server only and never sent to the browser. A `kit.LLM` interface hides the provider.
- R2 **Settings** per organization (`wl.ai_settings`): on/off; handover team (optional); tone; extra instructions
  (vertical pack text); confidence threshold (default 0.7); emergency words (defaults below, editable).
- R3 **Knowledge base** per organization: FAQ entries (question and answer) and text documents (pasted or uploaded as
  `.txt`/`.md`, up to 200 KB), split into chunks (~800 characters) and indexed with Postgres full-text search. Answers
  come only from this knowledge, and the sources used are kept in the run. **(assumption, Q1: full-text search instead
  of pgvector embeddings in this slice)**
- R4 **On `message.incoming`** (text only) when AI is on and no person owns the chat (no active agent transfer):
  1. **Emergency words** (deterministic, before any model call) → reply "If this is an emergency, call 000 now. I'm
     getting a person from the team to help you." and hand over.
  2. A pending choice from an earlier message (a time to book, a time to move to, "YES" to cancel) is completed.
  3. Otherwise **classify** the message: `book`, `reschedule`, `cancel`, `question`, `clinical`, `complaint`, `other`,
     with a confidence.
  4. `clinical`, `complaint`, `other`, or confidence below the threshold → a short holding reply and a **handover**.
  5. `question` → answer from the knowledge base only; no relevant knowledge, or the model says it can't answer from
     it → handover.
  6. `book` → the service (asked if unclear, matched by name, or the only one) → up to 5 next free times through the
     connector → "Reply with the number" → the chosen time is booked for the chat's contact.
  7. `reschedule` / `cancel` → the contact's next appointment through the connector → times to move to / "Reply YES to
     cancel" → done through the connector.
- R5 **Never clinical advice.** The prompts forbid it, clinical intent hands over, and every reply is checked for
  medical advice markers before sending (falls back to handover).
- R6 **Every AI reply starts with "🤖"** and says it's automated in the first reply of a conversation. **(assumption, Q2)**
- R7 **Only inside the 24-hour window:** the agent only replies to a message the patient just sent, so it's always
  inside the window; it never starts conversations or sends templates. Replies are service messages, not marketing.
- R8 **Runs** (`wl.ai_runs`): every handled message with intent, confidence, outcome (`answered`, `offered_times`,
  `booked`, `rescheduled`, `cancelled`, `handed_over`, `emergency`, `ignored`, `error`, `limit_reached` (6.2),
  `sent_link` (7.2: systems that book on their own page, such as Calendly, get a booking or reschedule link instead of
  offered times)), sources, prompt version,
  latency and tokens; no message text. Purged after 90 days.
- R9 **`POST /api/x/ai/reply`** runs the same pipeline for a contact and text and returns the reply (for a chatbot
  `api_call` node); `send: true` also sends it.
- R10 **Evals:** `internal/modules/ai/testdata/ai_evals.jsonl` holds de-identified clinic messages with the expected
  intent, whether an answer must come from the knowledge base, and whether it must hand over. A test runs it:
  - always (CI): the deterministic parts (emergency words, routing of each intent, handover rules) with a scripted
    model;
  - with `WL_AI_EVAL=1` and a provider key: the real model, failing below **90% intent accuracy** or **any clinical or
    emergency message not handed over** (100%). **(assumption, Q3)**
- R11 **Prompts are versioned in code** (`prompts.go`, `PromptVersion`), and the version is stored with each run.
- R12 **Screens:** AI agent → Settings (on/off, team, tone, instructions, threshold, emergency words; warns when no AI
  provider key is set), Knowledge (add FAQ, add document, list, delete), Activity (runs). Permissions `wl_ai_settings`
  (read, update), `wl_ai_knowledge` (read, create, delete), `wl_ai_runs` (read). Admin and manager all; agent runs
  read.

## Non-functional

- Every query scoped by organization; tested with a second organization.
- No message text, names or numbers in logs or runs.
- A model call has a 20-second timeout; on failure the agent hands over.
- Event handling idempotent (hooks de-duplicate; a run per message id).

## Out of scope (this slice)

- pgvector embeddings and website URL crawling (Q1).
- Media messages (images, voice notes).
- Group appointments, payments, waitlists.
- Proactive messages (reminders are Phase 5).

## Open questions

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Search: full-text now, embeddings later? Which embeddings provider? | Full-text (Postgres) |
| Q2 | How should AI replies be labelled? | "🤖" on every reply, "This is an automated assistant" on the first |
| Q3 | Eval pass mark | 90% intent accuracy; 100% of clinical and emergency messages handed over |
| Q4 | Emergency words | chest pain, can't breathe, cannot breathe, not breathing, unconscious, overdose, suicide, kill myself, severe bleeding, bleeding heavily, stroke, heart attack, seizure, anaphylaxis, emergency |
