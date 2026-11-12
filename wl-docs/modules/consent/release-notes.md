# Consent: release notes

## 2.2: Opt-in, STOP and consent history (first release)

Shipped in Weline Phase 2.2. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md). Works with Weline's existing "opted out of marketing" flag, which Campaigns already respect,
and with [Audiences](../audiences/release-notes.md).

### What's new

A **Consent** section in the sidebar with two pages, Consent and Consent settings.

- **STOP and START on WhatsApp:** when a patient replies with just STOP (or STOP ALL, STOP MESSAGES, UNSUBSCRIBE,
  OPT OUT, OPTOUT), they're opted out of marketing straight away:
  - Weline's "opted out of marketing" flag is set, so Campaigns skip them for marketing templates;
  - the contact gets an `opted-out` tag you can see in the chat and contact list;
  - they get one reply saying so, and that appointment messages still come.

  START, SUBSCRIBE or YES PLEASE opts them back in, with one confirmation. Case, spaces and punctuation don't matter,
  but the whole message must be the word, so "please don't stop my appointment" changes nothing. CANCEL isn't on the
  list, because patients use it for appointments. Sending STOP twice doesn't send a second reply.
- **Chatbot buttons as opt-in:** add the exact title of a chatbot button (for example "Yes, send me updates") as an
  opt-in phrase, and tapping it records an opt-in with source "button".
- **WhatsApp's own "Stop promotions":** already applied by Weline; it now also shows in the consent history (source
  "Meta") within 5 minutes.
- **Record a change by hand:** look up a contact by name or number, see their consent and history, and record an
  opt-in (with a note, such as "signed new patient form") or an opt-out, dated today or earlier.
- **Import opt-ins from a CSV file:** columns `phone`, `given_at` and optional `note`, up to 5,000 rows and 1 MB, with
  a description of the proof for the whole file. A preview shows what will happen before anything changes: how many
  rows will be applied, already opted in, unknown numbers (skipped, not created), invalid rows (with the reason) and
  people who opted out after the date in the file (they stay opted out). Australian numbers starting with 0 are
  accepted.
- **History:** every opt-in and opt-out with the contact (masked number), what changed, where it came from
  (keyword, button, staff, import, Meta), when it was given and recorded, who recorded it, and the note. Filter by
  action, source and dates. Records can't be edited or deleted.
- **Counts:** opted in, opted out and not recorded.
- **Settings:** the opt-out and opt-in word lists, opt-in button phrases, both replies (with `{business}` for your
  name and a preview), the tag, and two policies:
  - **Marketing only to people who opted in** (off by default). When on, Audiences leaves out contacts with no
    recorded opt-in for marketing templates. Audiences always shows how many have none.
  - **Service messages after STOP:** keep sending messages the patient asked for, such as appointment reminders
    (default), or stop all messages Weline starts itself. Later modules (reminders) follow this.
- **Audiences:** a new **Consent** rule (opted in, opted out, not recorded) and a "no opt-in" exclusion reason.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_consent_records` (counts, history, contact lookup; record a change, import) | read, create | read, create | read, create |
| `wl_consent_settings` (word lists, replies, tag, policies) | read, update | read, update | – |

### Upgrade notes

- Migration `consent/0001_create_consent` adds `wl.consent_settings`, `wl.consent_status`, `wl.consent_events` and
  `wl.consent_imports`, and grants the permissions above to the default roles. It adds no columns to Weline's
  tables. It changes contacts' existing `marketing_opt_out` flag and tags only when consent changes.
- **Existing opt-outs show up in the history within 5 minutes of the upgrade**, recorded as source "Meta" with the
  upgrade time as the date (Weline didn't keep the original date).
- Organizations created after the upgrade need the `wl_consent_*` permissions granted in the role editor (same as
  other modules).
- No WhatsApp templates needed: the replies are sent inside the 24-hour window the patient's message opens. Do add
  "Reply STOP to opt out" to the footer of every marketing template.

### Known limits

- **The chatbot also answers STOP.** If Weline's chatbot is on, a patient who sends STOP may get the chatbot's
  greeting or fallback as well as the confirmation, and in the middle of a flow STOP is taken as their answer. The
  opt-out still works. To be confirmed on staging and fixed later.
- "Marketing only to people who opted in" applies to campaigns made in Audiences. Campaigns created directly in
  Campaigns (pasted or imported lists) still skip opted-out contacts, but can't check for an opt-in.
- One marketing consent per contact; no separate consent for other purposes, no double opt-in.
- No opt-in from ads or a web form yet (ads come in step 2.3).
- Consent records are kept while the organization uses Weline; there's no deletion yet.
