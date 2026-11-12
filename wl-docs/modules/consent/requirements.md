# Consent: requirements

**For:** the business owner or manager responsible for marketing messages (clinic: practice manager), and reception
staff who take a patient's "yes" or "stop" at the desk or on the phone. Build guide step 2.2
([`../../build-guide/phase-2-messaging-platform.md`](../../build-guide/phase-2-messaging-platform.md)), direction
[ADR-002](../../adr/ADR-002-connect-dont-build-pms.md), follows [Audiences](../audiences/requirements.md).

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without customer input; each one is
listed under [Open questions](#open-questions).

Today Weline knows only one thing about consent: a contact's `marketing_opt_out` flag, which Meta sets when someone
taps "Stop promotions" in WhatsApp. Nobody records who **agreed** to marketing, a patient typing "STOP" is just a chat
message, and there's no record of when or how consent changed. Consent adds these. Australia's Spam Act 2003 requires
consent before commercial messages, a working unsubscribe, and proof of consent. WhatsApp's Business Messaging Policy
requires an opt-in before a business starts a conversation.

## What upstream already does (checked)

- **Campaigns skip opted-out contacts.** Weline's campaign worker marks a recipient failed ("Contact opted out of
  marketing messages") when the contact has `marketing_opt_out` and the template is MARKETING
  (`internal/worker/worker.go`). A single template send from the chat refuses the same case
  (`internal/handlers/messages.go`). This covers campaigns created outside Audiences (pasted or imported lists), so
  **no extra handling is needed for opt-outs**.
- **Meta's opt-out signal** (`user_preferences` webhook) sets `marketing_opt_out` true on "stop" and false on
  "resume" (`internal/handlers/webhook.go`). It raises **no event** modules can see, and records no history.
- **No STOP keyword handling**, no opt-in record, and no consent history.
- Staff can't change `marketing_opt_out` in Weline's screens (contact page and chat only show a badge).

## User stories

- As a patient, I reply STOP and stop getting promotions straight away, with a message saying so, and I can reply
  START to get them again.
- As a practice manager, I record when and how each patient agreed to marketing (a chatbot button, a keyword, a paper
  or online form, a list imported with proof), so that I can prove consent if asked.
- As a practice manager, I choose whether broadcasts go only to people who agreed, and Audiences and Campaigns never
  send marketing to people who said stop.
- As reception, when a patient tells me at the desk to stop (or says yes), I record it with a note, and it takes effect
  immediately.
- As a practice manager, I see every consent change: who, what, when, where it came from, and the evidence.

## Functional requirements

**Opt-out**
- R1 An incoming WhatsApp message whose whole text matches an **opt-out keyword** marks the contact opted out of
  marketing. Matching ignores case, surrounding spaces and punctuation, and needs the whole message to match, so
  "please don't stop my appointment" doesn't count. Default list: STOP, STOP ALL, STOP MESSAGES, UNSUBSCRIBE,
  OPT OUT, OPTOUT. **(assumption, Q1)** The list is configurable per organization. CANCEL is deliberately not in the
  list, because patients use it for appointments.
- R2 Opting out sets Weline's `marketing_opt_out` on the contact (so upstream's campaign worker and template send
  refuse marketing) and adds a configurable tag (default `opted-out`) so staff see it in the chat and contact list.
- R3 On a change of state, the contact gets **one** confirmation reply inside the conversation they just opened, for
  example: "You won't get offers or news from {business} on WhatsApp any more. Reply START to get them again.
  Appointment messages still come." The text is configurable. Repeating STOP when already opted out sends nothing.
- R4 Meta's own opt-out and resume signal is respected (upstream already applies it) and recorded in the history
  (R11) within 5 minutes, with source `meta`. **(assumption: detected by a sweep, see design)**

**Opt-in**
- R5 An incoming message matching an **opt-in keyword** (default START, SUBSCRIBE, YES PLEASE) or an **opt-in
  phrase** (the exact title of a chatbot button, such as "Yes, send me updates") opts the contact in: clears
  `marketing_opt_out`, removes the opt-out tag, records source `keyword` or `button`, and sends one configurable
  confirmation. **(assumption, Q1)**
- R6 Staff with permission record an opt-in or opt-out for one contact, with the date it was given (today or
  earlier) and a required note for opt-ins (for example "signed new patient form, 3 March 2026"). Source `staff`.
- R7 Staff import opt-ins from a CSV file (phone number, date given, optional note per row) with a required
  description of the proof for the whole file (for example "online newsletter sign-ups, Jan–Mar 2026, form text
  attached in practice records"). A preview shows how many rows match contacts, how many are unknown numbers, how
  many are invalid, and how many are already opted out. Applying records source `import`. Unknown numbers are
  skipped, not created. Contacts who opted out **after** the date in the file stay opted out. **(assumption, Q4)**
- R8 Consent is never inferred: booking an appointment, messaging the clinic, or being in an imported campaign list
  doesn't opt anyone in.

**Using consent**
- R9 A per-organization setting **"Marketing only to people who opted in"** (default **off**). **(assumption, Q2)**
  When on, Audiences leaves out contacts without a recorded opt-in for marketing templates (new exclusion reason
  `no_opt_in`), and its builder gains a **Consent** rule (opted in / opted out / not recorded). When off, Audiences
  still shows how many have no recorded opt-in.
- R10 A per-organization **service messages policy**: after STOP, either keep sending service messages the patient
  asked for, such as appointment reminders (default), or stop all non-reply messages. **(assumption, Q3)** Modules
  that send on their own (reminders, Phase 5) ask Consent before sending. Consent exposes this check to other
  modules; nothing sends on its own in this slice.

**Records and screens**
- R11 Every opt-in and opt-out is recorded with:
  - the contact;
  - the action and source (`keyword`, `button`, `staff`, `import`, `meta`; `ad` and `web_form` reserved for 2.3 and
    later);
  - when it was given and when it was recorded;
  - who recorded it (for staff and imports);
  - the evidence note, the WhatsApp message it came from (if any), and the state before.

  Records can't be edited or deleted in the app.
- R12 A **Consent** page shows:
  - counts: opted in, opted out, not recorded;
  - the history, filterable by action, source and date;
  - a contact lookup that shows one contact's current state and history and lets staff record a change (R6);
  - the CSV import (R7).
- R13 A **Consent settings** page edits the keyword and phrase lists, the two reply texts, the tag, R9 and R10. It
  also reminds managers to put "Reply STOP to opt out" in marketing templates (Meta's templates carry it in the
  footer).
- R14 Permissions:
  - `wl_consent_records`: read (counts, history, contact lookup) and create (record a change, import);
  - `wl_consent_settings`: read and update.

  On install, `admin` and `manager` get all; `agent` (reception) gets records read and create. **(assumption, Q5)**
- R15 Every screen shows loading, empty, error and forbidden states; read-only users see no edit controls.

## Non-functional

- STOP takes effect before the next send: the flag is set while handling the event (the Spam Act allows 5 working
  days; we aim for seconds). Handling is idempotent (the hooks receiver de-duplicates; state changes are no-ops when
  already in that state).
- Every query is scoped to the organization and tested with a second organization.
- No phone numbers, names or message text in logs. History keeps the matched keyword, never the full message.
- Strings under `wl.consent.*`. Upstream code is not changed: the module updates `contacts.marketing_opt_out` and
  `contacts.tags` through upstream's model, as Weline's own handlers do.

## Out of scope (this slice)

- Ad leads as an opt-in source (step 2.3 records them through this module) and a public web sign-up form, including a
  marketing tick box on the booking page.
- Consent for purposes other than marketing (for example research or recalls as their own category); one marketing
  consent per contact for now.
- Double opt-in (confirming by a second message), and consent per WhatsApp number when an organization has several:
  consent is per contact, and contacts are already per account.
- Enforcing "opted in only" (R9) for campaigns created outside Audiences. Upstream's worker only knows opt-out;
  enforcing opt-in there would need an upstream change. Pasted or imported campaign lists stay the manager's
  responsibility, and the settings page says so.
- Automatic deletion of consent records (see Q6).

## Open questions

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Which words should opt out and opt in? Do patients use any others (for example "remove me", "no more")? | Out: STOP, STOP ALL, STOP MESSAGES, UNSUBSCRIBE, OPT OUT, OPTOUT. In: START, SUBSCRIBE, YES PLEASE. Editable per clinic. |
| Q2 | Should marketing go only to people with a recorded opt-in? (Strictest; at first almost nobody has one, so broadcasts would reach no one until opt-ins are collected or imported.) | Off, with the "not recorded" count shown in Audiences |
| Q3 | After STOP, should appointment reminders and other service messages keep coming? | Yes: STOP covers marketing only; the reply says so |
| Q4 | When importing opt-ins, should unknown numbers become new contacts? | No, they're skipped and counted |
| Q5 | May reception record opt-ins and opt-outs, or only managers? | Reception may record both (with a note for opt-ins) and can't change settings |
| Q6 | How long must consent records be kept? | While the organization uses Weline; no deletion in this slice |
| Q7 | Does the clinic already hold signed consent (new patient forms) that should be imported on day one? | Supported through the CSV import |
