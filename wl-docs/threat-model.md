# Weline threat model

One page, updated by prompt C whenever a module adds a public endpoint, a new kind of data or an external
integration. Architecture: [`architecture.md`](architecture.md). Data: [`data-model.md`](data-model.md).
STRIDE: **S**poofing, **T**ampering, **R**epudiation, **I**nformation disclosure, **D**enial of service,
**E**levation of privilege. Last reviewed: 2026-11-13 (Phase 4, AI agent).

## Assets

| Asset | Where | Why it matters |
| --- | --- | --- |
| Patient contact data (phone, name, appointments, CRM profile) | `contacts`, `wl.booking_appointments` and history, campaign recipients (`bulk_message_recipients`, written by Audiences with names and rendered template values), future `wl.*` tables | Health-related personal information under the Privacy Act 1988 / APPs |
| Consent records | `wl.consent_events`, `wl.consent_status`, opt-in CSVs (parsed rows in Redis for 15 minutes) | Proof of consent under the Spam Act 2003 and WhatsApp policy; staff notes may mention forms. Tampering could send marketing to people who said stop, or remove the proof |
| Ad attribution | `wl.ads_arrivals`, contact tags `from-…` and `campaign-…` | Which ad or offer a patient answered (for example a skin check offer), which can hint at a health interest; marketing data, not clinical |
| Messages | `messages.content`, media storage | May contain health details; never logged |
| WhatsApp access tokens, app secrets, PINs | `whatsapp_accounts` (encrypted) | Send as the clinic, read its traffic |
| Webhook signing secrets | `webhooks.secret` (plain text, upstream design) | Forge events into modules |
| API keys, JWT secret, encryption key | `api_keys`, env (`WHATOMATE_JWT__SECRET`, `WHATOMATE_APP__ENCRYPTION_KEY`) | Full API access; decrypt every secret |
| Clinic setup (practice module) | `wl.practice_*` | Practitioner names, roles and AHPRA numbers (public-register data), clinic contact details, opening hours and the booking slug. Not patient data, but tampering breaks booking and could send patients to the wrong place or time |
| Captcha secret | Render environment (`WELINE_TURNSTILE_SECRET`) | Without it bookings are refused; leaked, bots can pass the robot check |
| Connected system credentials | `wl.connector_accounts.credentials` (encrypted with `encryption_key`) | A Cliniko key opens the clinic's patient records in Cliniko with the permissions of the user who made it |
| AI keys (planned) | config | Access to the AI account |

## Entry points and threats

| Entry point | S | T | R | I | D | E | Mitigation and where it lives |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **Login** (`/api/auth/login`, SSO, cookies) | Stolen or guessed password | – | – | Account enumeration | Brute force | – | bcrypt hashes, httpOnly `whm_access` cookie + CSRF double-submit (`internal/middleware/csrf.go`), login rate limit (`cmd/whatomate/main.go`, `rate_limit.*`; enabled on Render), secure cookies in production. Upstream code. |
| **`/api/x/*` module routes** | Request without a session | – | Who changed what | Another org's rows | Large queries | User without the permission; permission from a different org | `AuthWithDB` on all `/api` (`cmd/whatomate/main.go`); `kit.Guard` checks a `wl_` permission **in the active org** (`internal/modules/kit/request.go`); services filter every query by `orgID` (e.g. `internal/modules/example/service.go`); `kit.SendError` hides internal errors; list limits. Practice (`/api/x/practice`) adds composite `(organization_id, id)` foreign keys so the database refuses cross-org links, separate read and write permissions (reception read-only by default), and per-endpoint request structs that ignore `organization_id`/`id` in bodies (`internal/modules/practice/handlers_test.go`). Booking (`/api/x/booking`) adds the same composite keys into the practice tables, a Postgres exclusion constraint against double booking (tested with ten parallel requests), append-only status history with the acting user, and patient search by POST so names and numbers never sit in URLs. |
| **Public booking page** `/book/<slug>`, `/x/booking/public/<slug>/…` (no login, outside `/api`) | Fake or spam bookings; booking under someone else's mobile | Tampered slot, practitioner or price | – | Other patients' bookings, internal ids, whether a mobile is already known | Booking spam, slot scraping | – | Cloudflare Turnstile verified server-side on every booking, refused (503) until keys are set; rate limits per hashed IP (60 reads/min, 5 bookings/h) and per hashed mobile (3/day), failing open if Redis is down; the server recomputes the slot (client sends ids and a start time only; duration and end come from the service); responses carry clinic name, locations, services, practitioner names and roles, free times and the caller's own reference only, the same whatever the mobile; strict CSP with a nonce, `frame-ancestors 'none'`, `credentials: 'omit'`; unknown slug 404. Tested in `internal/modules/booking/handlers_test.go`. See open risk 10. |
| **`POST /x/hooks/weline`** (module event receiver) | Forged events | Payload altered in transit | – | – | Flooding | Event injected into another org | Per-org random token in `X-Weline-Hook-Token` (stored in the webhook's custom headers), constant-time compare, required on every delivery; plus HMAC-SHA256 of the raw body with the org's secret **whenever Weline sends a signature** (upstream omits it on cached deliveries, see open risk 1); `X-Weline-Org` is proven by the token; HTTPS only; stale (>1 h) or future deliveries rejected; Redis de-duplication; 401 before any work (`internal/modules/hooks/receiver.go`). Outside `/api`, so no session needed (by design). |
| **Chatbot `api_call` nodes** → module endpoints | Calls made with someone's API key | Flow edited to call another URL | – | API key visible in flow config | – | API key with more rights than needed | Calls go through Weline's SSRF-safe client (`internal/handlers/webhooks.go` `SSRFSafeDialer`); module endpoints still need auth + `wl_` permission. **Open risk:** the key sits in the flow config. Booking's chatbot endpoints only book for the phone number of the conversation's contact in the key's org, and offers expire after 30 minutes. |
| **Audiences** (`/api/x/audiences`) → draft campaigns | – | Rules crafted to inject SQL or read another org's data | Who created which campaign for whom | Contacts' phone numbers; other orgs' campaigns or templates; contact attributes inferred from counts | Expensive rules, huge audiences | Creating campaigns without Weline's campaign permission | Rules are a closed set of fields and operators compiled to **parameterised SQL**; contact field names must match `^[a-zA-Z0-9_]{1,50}$`, LIKE wildcards escaped (injection tests in `internal/modules/audiences/service_test.go`); every query filtered by org, and campaign and template ids checked to be the org's; the preview returns masked numbers only (first 2 and last 3 digits); creating a campaign needs `wl_audiences_sends:create` **and** upstream `campaigns:write`, and only makes a **draft** (starting needs `campaigns:execute` in Campaigns); `confirm_count` must match or the request is refused; at most 20 conditions and 10,000 recipients; each send is recorded with its creator, rules snapshot and exclusions (`wl.audiences_sends`). Opted-out contacts are left out for marketing templates, and upstream's sender checks again. See open risks 12–13. |
| **Consent** (`/api/x/consent`) and STOP/START from `message.incoming` | Forged `message.incoming` events flipping consent | A CSV or staff record that opts in people who never agreed | Who recorded which opt-in, on what proof | Other orgs' contacts or history; full numbers | Huge CSVs; event floods | Reception changing settings | Keywords act only on the conversation's own contact, found inside the event's org (the hooks receiver authenticates the org); a message must be the whole keyword; state changes and the reply happen once (`TestDuplicateEventChangesOnce`). Staff and import changes always record source `staff`/`import` and the caller as recorder, whatever the body says; opt-ins need a note, imports a proof description, and an import never opts in someone who opted out later or creates contacts. History is append-only in code (see open risk 15). Numbers masked in responses; contact search by POST. CSV capped at 1 MB and 5,000 rows. Settings need `wl_consent_settings:update` (not reception). Every query filtered by org; composite keys stop cross-org links. See open risks 12, 14, 15. |
| **Ads** (`/api/x/ads`) and code matching on `message.incoming` | Anyone typing a code to inflate counts or give themselves a tag | Links pointing patients to another number or text; tags crafted to break Audiences | Who created or changed an entry point | Other orgs' entry points or arrivals; full numbers | Event floods; huge reports | Reception viewing attribution | Codes need a letter and a digit and match only as whole words; one arrival per contact, entry point and day; only verified hooks deliveries count. Numbers are digits only (8–15, country code), messages ≤ 500 characters and must hold the code, and the link is built by the server with URL encoding. Tags are letters, digits, `-` and `_` (≤ 50), and the module only adds tags. Creator and last editor recorded. Every query filtered by org, composite keys, contact looked up inside the event's org; numbers masked; no message text stored or logged. Active codes cached in memory, one small transaction per match; reports capped at 366 days (about 0.6 s for 50,000 arrivals). Reception has no `wl_ads_entrypoints` by default. See open risk 16. |
| **Connectors** (`/api/x/connectors`) and the provider resolver | Someone saving a key for another clinic's system | Credentials or provider swapped to send bookings elsewhere | Who changed the connected system | Credentials read back; another org's provider used | A slow provider hanging WhatsApp | Managers or reception changing the system | Only `wl_connectors_settings:update` (admins) can choose or test; the connection test runs before anything is saved; credentials encrypted (`enc:`), refused without an encryption key, never returned or logged; `updated_by_id` recorded; providers built bound to one organization's row; 10-second test timeout and typed errors turned into the flow's error branch; the chatbot refuses offers made before a provider switch. See open risk 18. |
| **AI agent** (`message.incoming`, `/api/x/ai`) and the AI provider | Forged messages acting as a patient | Prompt injection to make it act or reveal | What it did and why | Patient messages and clinic knowledge sent to the AI provider; the provider key | Message floods running up AI cost | The model taking actions | Only verified hook deliveries; emergency words before any model call; the model only classifies and words answers from given passages, parsed as strict JSON, never follows instructions in the message; actions need the patient's numbered choice and run through the connector for the chat's own contact; clinical, complaint, unclear or low-confidence messages hand over; replies screened for advice markers; one run per message id; 20-second timeouts; key decrypted server-side, never returned or logged; runs keep no text and are purged after 90 days. See open risk 20. |
| **Reminders** (sweep, `message.incoming` replies, `/api/x/reminders`) | Forged replies cancelling someone's appointment | Rules pointed at a marketing template to bypass consent | Who changed a rule; who sent recalls | A reminder to the wrong patient; other orgs' rules or sends | A sweep sending twice; a huge recall send | Reception sending recalls | Only verified hook deliveries; a reply acts only on the conversation's own contact's latest unanswered reminder for a future appointment, and must be the whole word; one send per (rule, appointment, start time) by a unique key claimed before sending (a crash means a miss, never a duplicate); sweep on one instance (Redis lock); templates must be approved; reminders follow the service-message consent rule and recalls need marketing consent; recalls are sent by a person, once per customer, rule and month; contacts come from the connector's mobile for that customer; rule changes and recall sends audited; no text or numbers stored or logged; recalls need `wl_reminders_recalls` (not reception). See open risk 21. |
| **Sign-up** (`/x/onboarding/signup`, `/x/onboarding/public/signup`) and new-organization grants | Bots creating organizations; someone signing up with another person's email | – | Who signed up when | Whether an email has an account | Mass sign-ups filling the database | A sign-up landing in or reading another organization | Off unless `WELINE_SIGNUP=open`; Turnstile on every attempt (refused without keys); 5 an hour per hashed address; writes only the new organization's rows in one transaction using upstream's own role seeding; the person is admin of that organization only; password bcrypt-hashed, never logged; CSP with a nonce on the page; audit entry `signed_up`. Module permissions granted once per organization to named system roles only. See open risk 22. |
| **Billing** (`/api/x/billing`, Stripe webhook `/x/billing/stripe/webhook`, outbound to `api.stripe.com`) | Forged webhooks granting a plan | Events replayed or reordered to restore a cancelled plan | Who started checkout or opened the portal | Stripe secret key; invoices of another organization | Webhook floods | Managers buying; one organization's events changing another | Off unless `WELINE_BILLING=on`; webhook signature (HMAC-SHA256, 5-minute tolerance, constant-time) over the raw body, refused without a secret; each event id applied once; events older than the last applied ignored; organization found by subscription id, our metadata or customer, exactly one; Stripe's hosted pages take card details (none in Weline); secret key only in the environment, never logged or returned; buying needs `wl_billing_plan:update` (admins); writes audited. Limits fail closed for metered work but never touch reminders, replies or STOP. See open risk 23. |
| **Meta webhook** (`/api/webhook`, upstream) | Forged WhatsApp events | – | – | – | Flooding | – | Upstream verifies `X-Hub-Signature-256` **only when the account has an App Secret stored**. Set it for every account. |
| **Cliniko API** (outbound, `api.<shard>.cliniko.com`) and the phone index sync | Spoofed responses; a key pointed at another account | – | Sync log | The key in logs or errors; patient names and numbers kept by Weline | Cliniko's 200/min limit; a slow Cliniko | – | HTTPS to the shard derived from the key (validated against Cliniko's format); `links.next` only followed inside the account's API URL; Basic auth key never logged or put in errors (errors keep Cliniko's status and a short message, never bodies); 10-second timeouts; one wait on a 429 when the reset is within 10 s, otherwise `RateLimited`; only ids and salted phone hashes kept (`wl.connector_links`), free times cached in memory for a minute; sync on one instance with a Redis lock. See open risk 19. |
| **Outbound sends** (`kit.Sender`) | – | – | Who sent it | Message to the wrong patient | – | Sending as another org | Contact and account resolved inside the org (`internal/modules/kit/sender.go`); messages are stored and visible in the inbox; templates must be approved. **Open risk:** module sends carry no `sent_by_user_id` yet. |

## Cross-cutting controls

- Org isolation is tested with a second organization in every module's test suite.
- No phone numbers, names or message text in logs (review checklist in `CLAUDE.md`). Keep `WHATOMATE_APP__DEBUG`
  `false` wherever there is real data: debug mode logs every SQL query with its values, including phone numbers.
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
   closed (no access), not open: in a new org even admins see no Clinic menu until granted.
6. **Render free tier (plan in ADR-004)**: data is hosted in Oregon, not Australia, and free Postgres expires. Production
   needs an Australian region before real patient data (SDLC "Compliance").
7. **Partly closed (3.3): module changes are audited; reads aren't.** `wl.audit_events` records changes to the
   connected system, clinic setup, consent settings and ads entry points, with the user (`wl-docs/modules/audit`).
   Still open: who viewed patient data, and credential use per request. Originally: **No audit trail for module reads/writes yet.** Upstream's audit log isn't wired into modules; to do
   before patient data arrives (SDLC privacy checklist). Since Phase 1.1 this includes clinic setup: who
   changed hours, closures, services or the booking slug isn't recorded (only `updated_at`).
8. **Module sends aren't attributed.** `kit.Sender` doesn't set `sent_by_user_id`, so a staff-triggered
   module message can't be traced to the person from the message row. Add an optional user to the sender
   when the first module sends on a user's behalf (Phase 1.2/2).
9. **Mitigated (3.3, ADR-003): identifiers are masked in the process's output** by a scrubber Weline starts with its
   modules (`WELINE_SCRUB_LOGS=off` turns it off). Still open: lines written before modules start, and
   names inside SQL when debug mode is on; the real fix is upstream. Originally: **Upstream logs patient identifiers.** Recipient phone numbers on every send (`Text message sent …
   phone=…`), and sender numbers and profile names on every incoming message (`Received message from=…`,
   `profile_name=…`), at info level, so they land in the hosting provider's logs. Needs a log-level change
   or log scrubbing before real patients; raise upstream.
10. **Public bookings can be made under someone else's mobile.** The captcha and per-mobile limits stop bulk abuse,
    but a person can book (and so create a contact) with another person's number. The clinic confirms at arrival;
    add mobile verification (WhatsApp one-time code) if abuse appears.
11. **Closed (3.3): a daily job deletes built-in appointments and their history 7 years after the appointment**
    (`booking.DeleteExpired`, one instance via a Redis lock). Weline's messages (upstream table) still follow
    upstream's behaviour: no deletion; deciding a messages policy needs your input. Originally: **Booking records are patient data without a deletion job yet.** The retention rule (7 years after the
    appointment) is recorded in `wl-docs/modules/booking/design.md`; the job must exist before real patients.
12. **Marketing to people with no recorded opt-in is still allowed by default.** Since 2.2 (Consent), STOP is
    handled and opt-ins are recorded with proof, but "Marketing only to people who opted in" is **off** by default
    (requirements Q2), and it only applies to campaigns built in Audiences: Campaigns created directly (pasted or
    imported lists) skip opted-out contacts but can't check for an opt-in without an upstream change. A manager can
    still send marketing to people who never agreed, which risks complaints, a lower Meta quality rating and Spam
    Act 2003 issues. Turn the setting on once opt-ins are imported, and build marketing campaigns in Audiences.
13. **Counts reveal facts about contacts the viewer may not otherwise see.** Anyone with
    `wl_audiences_segments:read` (reception by default) can count contacts by tag, contact field value or campaign
    response across the whole organization, and see a masked sample with names. That is aggregate access beyond
    an agent's assigned chats. Remove `read` from roles that shouldn't see it (Q1).
14. **The chatbot also answers STOP.** Upstream's chatbot processor handles every incoming message, so with the
    chatbot on, a patient who sends STOP can get the greeting, fallback or AI reply as well as Consent's
    confirmation, and inside a flow STOP is taken as the answer to the current step (found by reading
    `internal/handlers/chatbot_processor.go`; to confirm on staging). The opt-out still takes effect. Fix needs
    upstream to let a module mark a message as handled, or a chatbot keyword rule for the opt-out words.
15. **The consent history is append-only in code, not in the database.** The module has no update or delete for
    `wl.consent_events`, but anyone with write access to the database could change or remove proof of consent.
    Before real patients, consider a trigger refusing `UPDATE`/`DELETE` on the table, or a database role for the
    app without those rights on it.
16. **Ad attribution is indicative, not proof.** Anyone who sees a code can type it, so counts and `from-…` tags can
    be inflated (low impact: marketing tags and counts only). Patients who delete the code aren't counted, so
    counts are also a minimum. Meta's own referral data, which can't be faked this way, is out of reach without an
    upstream change (`wl-docs/modules/ads/spike.md`, option b declined).
17. **Upstream checks business hours against the server's clock.** Its out-of-hours reply compares the chatbot's
    business hours with `time.Now()` in the server's time zone, and staging sets `TZ=UTC` (`render.yaml`). A Sydney
    clinic's out-of-hours reply would go out 10–11 hours off, telling patients the clinic is closed when it's open
    and the reverse. Weline's own `IsOpen` (Practice) uses the clinic's time zone, so the two disagree until the
    server's `TZ` is the clinic's zone. Set `TZ=Australia/Sydney` while all clinics share one zone; supporting several
    zones needs upstream to store a time zone with the business hours.
18. **Connected system credentials can be decrypted with the database and the encryption key.** A Cliniko key gives
    whoever holds it the access of the Cliniko user it belongs to, including patient records. It's encrypted at rest
    with Weline's `encryption_key` (as WhatsApp tokens are), so anyone with both the database and the key can read
    it. Keep the key out of the repository and backups, ask clinics to create a dedicated Cliniko user with the least
    access, and add the audit trail for credential use (step 3.3).
19. **Weline acts in a clinic's Cliniko with the full rights of the key's user.** Cliniko keys can't be scoped, so a
    key from an admin user lets Weline (and anyone who extracts the key) read and change everything in that Cliniko
    account. Ask clinics to create a dedicated "Weline integration" user with the lowest role that can see the diary
    and patients, and to revoke the key when they disconnect. Weline only reads names and phone numbers of patients
    and never stores them.
20. **Patient messages go to a third-party AI provider.** The agent sends the patient's message (and clinic knowledge)
    to the provider chosen in Settings → Chatbot → AI (OpenAI, Anthropic or Google), outside Australia unless the
    provider offers otherwise. Before real patients: choose a provider and plan with zero data retention and suitable
    terms, mention it in the clinic's privacy notice, and confirm with the clinic. The eval set still needs a real-model
    run to confirm the 100% hand-over of clinical and emergency messages beyond the word list.
21. **Reminders reach whoever holds the mobile in the clinic's system.** A wrong or old number in Cliniko gets the
    patient's first name, service and time. The clinic keeps numbers current; keep template variables to what a
    reminder needs. Cliniko's API has no confirmation field, so a Confirm reply is recorded in Weline only.
22. **Sign-up doesn't verify email addresses.** Someone can sign up with an address they don't own (the real owner
    later finds it taken), and the 409 tells a visitor that an address has an account (as upstream's register already
    does). Add email verification when Weline can send email (onboarding Q1).
23. **Usage checks aren't atomic with the work.** Two broadcasts started at the same moment can both pass the check
    and go one campaign over the month's allowance; AI replies likewise by a message or two. Acceptable for monthly
    allowances; if it matters, take a per-organization Redis lock around the check and the insert.