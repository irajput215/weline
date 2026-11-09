# Weline threat model

One page, updated by prompt C whenever a module adds a public endpoint, a new kind of data or an external
integration. Architecture: [`architecture.md`](architecture.md). Data: [`data-model.md`](data-model.md).
STRIDE: **S**poofing, **T**ampering, **R**epudiation, **I**nformation disclosure, **D**enial of service,
**E**levation of privilege. Last reviewed: 2026-11-09 (Phase 0, after the step 0.9 staging check).

## Assets

| Asset | Where | Why it matters |
| --- | --- | --- |
| Patient contact data (phone, name, appointments, CRM profile) | `contacts`, future `wl.*` tables | Health-related personal information under the Privacy Act 1988 / APPs |
| Messages | `messages.content`, media storage | May contain health details; never logged |
| WhatsApp access tokens, app secrets, PINs | `whatsapp_accounts` (encrypted) | Send as the clinic, read its traffic |
| Webhook signing secrets | `webhooks.secret` (plain text, upstream design) | Forge events into modules |
| API keys, JWT secret, encryption key | `api_keys`, env (`WHATOMATE_JWT__SECRET`, `WHATOMATE_APP__ENCRYPTION_KEY`) | Full API access; decrypt every secret |
| Connector and AI keys (planned) | `wl.connector_accounts`, config | Access to the clinic's practice system and AI account |

## Entry points and threats

| Entry point | S | T | R | I | D | E | Mitigation and where it lives |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Login** (`/api/auth/login`, SSO, cookies) | Stolen or guessed password | – | – | Account enumeration | Brute force | – | bcrypt hashes, httpOnly `whm_access` cookie + CSRF double-submit (`internal/middleware/csrf.go`), login rate limit (`cmd/whatomate/main.go`, `rate_limit.*`; enabled on Render), secure cookies in production. Upstream code. |
| **`/api/x/*` module routes** | Request without a session | – | Who changed what | Another org's rows | Large queries | User without the permission; permission from a different org | `AuthWithDB` on all `/api` (`cmd/whatomate/main.go`); `kit.Guard` checks a `wl_` permission **in the active org** (`internal/modules/kit/request.go`); services filter every query by `orgID` (e.g. `internal/modules/example/service.go`); `kit.SendError` hides internal errors; list limits. |
| **Public booking page** (planned, Phase 1.2) | Fake bookings | Tampered slot or price | – | Leaking other patients' slots or names | Booking spam | – | To build: no login, rate limit + captcha, server-side slot validation, return only a reference, APP 5 collection notice. |
| **`POST /x/hooks/weline`** (module event receiver) | Forged events | Payload altered in transit | – | – | Flooding | Event injected into another org | Per-org random token in `X-Weline-Hook-Token` (stored in the webhook's custom headers), constant-time compare, required on every delivery; plus HMAC-SHA256 of the raw body with the org's secret **whenever Weline sends a signature** (upstream omits it on cached deliveries, see open risk 1); `X-Weline-Org` is proven by the token; HTTPS only; stale (>1 h) or future deliveries rejected; Redis de-duplication; 401 before any work (`internal/modules/hooks/receiver.go`). Outside `/api`, so no session needed (by design). |
| **Chatbot `api_call` nodes** → module endpoints | Calls made with someone's API key | Flow edited to call another URL | – | API key visible in flow config | – | API key with more rights than needed | Calls go through Weline's SSRF-safe client (`internal/handlers/webhooks.go` `SSRFSafeDialer`); module endpoints still need auth + `wl_` permission. **Open risk:** the key sits in the flow config. |
| **Meta webhook** (`/api/webhook`, upstream) | Forged WhatsApp events | – | – | – | Flooding | – | Upstream verifies `X-Hub-Signature-256` **only when the account has an App Secret stored**. Set it for every account. |
| **Connector syncs** (planned, Phase 4) | Spoofed practice-system responses | – | Sync log | Credentials leak | Rate limits (Cliniko 200/min) | – | To build: HTTPS only, encrypted credentials in `wl.connector_accounts`, `wl.connector_syncs` log, backoff on 429, no live calls in CI. |
| **Outbound sends** (`kit.Sender`) | – | – | Who sent it | Message to the wrong patient | – | Sending as another org | Contact and account resolved inside the org (`internal/modules/kit/sender.go`); messages are stored and visible in the inbox; templates must be approved. **Open risk:** module sends carry no `sent_by_user_id` yet. |

## Cross-cutting controls

- Org isolation is tested with a second organization in every module's test suite.
- No phone numbers, names or message text in logs (review checklist in `CLAUDE.md`).
- The seam guard keeps security-relevant upstream code unmodified, so upstream fixes merge cleanly.
- Secrets are encrypted at rest with `encryption_key` (upstream `internal/crypto`), except webhook secrets.

## Open risks

1. **Upstream signs webhooks only on a cache miss.** `getWebhooksCached` caches webhooks in Redis as JSON
   and `models.Webhook.Secret` is tagged `json:"-"`, so deliveries made from the cache carry no
   `X-Webhook-Signature` (found on staging 2026-11-09; reproduced by
   `TestEndToEnd_SecondDispatchFromWelineCache`). Our receiver therefore relies on the per-org token over
   HTTPS, with no body signature on most deliveries. The same bug silently disables signing for every
   Weline webhook customers configure. Fix belongs upstream (cache the secret, or reload it before
   signing); report it.
2. **Webhook secrets and the hook token are stored in plain text** (upstream design), and the token is
   visible to anyone who can view webhooks in Weline's settings. Anyone with that access can forge module
   events. Mitigation: restrict DB access and the webhooks permission.
3. **Meta webhook signatures are optional upstream.** If a WhatsApp account has no App Secret, forged Meta
   events are accepted and flow on to modules. Every account must have its App Secret set.
4. **API keys in chatbot flow configs.** Keys used by `api_call` nodes are readable by anyone who can edit
   flows, and API keys carry the full rights of their user. Use a dedicated low-permission user per org.
5. **Module permissions don't reach organizations created later.** System roles of new orgs come from the
   upstream seed, so admins must grant `wl_` permissions by hand (`kit.SeedPermissions` note). This fails
   closed (no access), not open.
6. **Render free tier**: data is hosted in Oregon, not Australia, and free Postgres expires. Production
   needs an Australian region before real patient data (SDLC "Compliance").
7. **No audit trail for module reads/writes yet.** Upstream's audit log isn't wired into modules; to do
   before patient data arrives (SDLC privacy checklist).
8. **Module sends aren't attributed.** `kit.Sender` doesn't set `sent_by_user_id`, so a staff-triggered
   module message can't be traced to the person from the message row. Add an optional user to the sender
   when the first module sends on a user's behalf (Phase 1.2/2).
9. **Upstream logs patient identifiers.** Recipient phone numbers on every send (`Text message sent …
   phone=…`), and sender numbers and profile names on every incoming message (`Received message from=…`,
   `profile_name=…`), at info level, so they land in the hosting provider's logs. Needs a log-level change
   or log scrubbing before real patients; raise upstream.
