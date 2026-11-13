# AI agent: test plan

Go tests use a scripted `kit.LLM` (returns given JSON per prompt), a stub connector provider, a fake hand-over and a
fake sender, against real Postgres and Redis.

| Case | Covers |
| --- | --- |
| LLM clients send the right request shape (Anthropic, OpenAI, Google) to an `httptest` server and read the answer and token counts; key from Weline's settings decrypted; no key → "AI provider not set up" | R1 |
| Settings defaults, validation (threshold 0.3–0.95, instructions ≤ 4,000, emergency words list) | R2 |
| Knowledge: FAQ and document chunking, search ranks the right chunk, delete by source, other org's knowledge never found | R3 |
| Pipeline: AI off / active transfer / non-text → ignored; emergency → 000 reply and handover without a model call; clinical, complaint, low confidence → handover; question with knowledge → labelled answer with sources; question without knowledge → handover; model says can't answer → handover | R4, R5, R6 |
| Booking: book → service matched → 5 times offered → "2" books the second for the chat's contact; reschedule → next appointment → "1" moves it; cancel → "YES" cancels; stale or unknown choice → re-asks | R4 |
| A reply containing advice markers is replaced by a handover | R5 |
| Runs recorded without text; one run per message id; purge after 90 days | R8 |
| `POST /reply` returns the reply; permissions; second organization isolated | R9, R12 |
| Evals: scripted mode always; real model with `WL_AI_EVAL=1` | R10 |
| Prompt version stored | R11 |
| Vitest store states; Playwright: settings and knowledge pages, reception sees activity only | R12 |
