# AI agent: API

Under `/api/x/ai`, Weline login (or API key) required, permission checked in the active organization.

| Method and path | Permission | Notes |
| --- | --- | --- |
| `GET /settings` | `wl_ai_settings:read` | `{ settings, provider: { name, model, key_set } }` (provider from Chatbot → AI; never the key) |
| `PUT /settings` | `wl_ai_settings:update` | `{ enabled, handover_team_id, tone, instructions, confidence_threshold, emergency_words }` |
| `GET /knowledge` | `wl_ai_knowledge:read` | `{ items: [ { source_id, kind, title, chunks, created_at } ] }` |
| `POST /knowledge` | `wl_ai_knowledge:create` | `{ kind: "faq", question, answer }` or `{ kind: "document", title, text }` (≤ 200 KB) → `{ item }` |
| `DELETE /knowledge/{id}` | `wl_ai_knowledge:delete` | by `source_id` |
| `GET /runs?before=` | `wl_ai_runs:read` | newest first, 50 a page: `{ runs: [ { id, contact: { id, name, phone_masked }, intent, confidence, outcome, sources, prompt_version, latency_ms, created_at } ], next_before }` |
| `POST /reply` | `wl_ai_runs:read` and `wl_ai_settings:read` | `{ contact_id, text, send? }` → `{ reply, intent, outcome, handed_over }`; runs the pipeline for a chatbot `api_call` node |
