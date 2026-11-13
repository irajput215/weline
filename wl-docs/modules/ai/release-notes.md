# AI agent: release notes

## Phase 4: the WhatsApp AI agent (first release)

Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md). API: [`api.md`](api.md).

### What's new

**AI → AI agent**, with three tabs:

- **Settings:** turn the agent on; choose a tone (friendly, formal, brief) and how sure it must be before acting (below
  that it passes the chat to your team); add instructions; edit the emergency words. It uses the AI provider, model and
  key from Settings → Chatbot → AI.
- **Knowledge:** add questions and answers, and documents (paste text or a .txt/.md file). The agent only answers from
  this; if the answer isn't there, your team gets the chat.
- **Activity:** what the agent did with each message (no message text), kept for 90 days.

**What patients get on WhatsApp:**

- Answers to questions about the clinic, marked with 🤖, the first one saying it's an automated assistant.
- **Booking:** "can I book?" → which appointment → the next 5 free times → reply with a number → booked, in your
  connected system (Weline's booking or Cliniko).
- **Moving and cancelling** their next appointment the same way ("Reply YES to cancel").
- **Anything about their health, complaints, or anything unclear goes to a person.** Emergency words ("chest pain",
  "can't breathe", …) get "call 000" and a person straight away, before the AI is even asked. Replying HUMAN always
  reaches a person.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_ai_settings` | read, update | read, update | – |
| `wl_ai_knowledge` | read, create, delete | read, create, delete | – |
| `wl_ai_runs` | read | read | read |

### Upgrade notes

- Migration `ai/0001_create_ai` adds `wl.ai_settings`, `wl.ai_knowledge` and `wl.ai_runs`.
- Set an AI provider, model and key in Settings → Chatbot → AI, and `WHATOMATE_APP__ENCRYPTION_KEY` on the server.
- Turn off the chatbot's own AI and fallback reply while the agent is on.

### Known limits

- Search is full-text, so very differently worded questions may miss; embeddings come later.
- Text messages only (no voice notes or photos).
- Message text and knowledge passages go to the AI provider you chose; pick one whose data terms suit patient data.
