# Audiences: requirements

**For:** the business owner or manager who sends broadcasts (clinic: practice manager; later any small business),
and the team who reviews results. Build guide step 2.1 ([`../../build-guide/phase-2-messaging-platform.md`](../../build-guide/phase-2-messaging-platform.md)),
direction [ADR-002](../../adr/ADR-002-connect-dont-build-pms.md), background
[`../../product/wati-comparison.md`](../../product/wati-comparison.md).

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without customer input; each one is
listed under [Open questions](#open-questions).

Today Weline's campaigns send a template to a list you import (phone numbers pasted or uploaded). Audiences adds the
missing half: build the list from the contacts you already have, and follow up with the people a campaign didn't
reach. Sending, scheduling, pausing and per-recipient delivery stay in Weline's existing Campaigns.

## User stories

- As a practice manager, I build a segment from my contacts, e.g. "tag *physio* and messaged us in the last 90 days",
  see how many people it reaches, and save it, so that I can send the right message to the right patients.
- As a practice manager, I turn a segment into a campaign with a chosen template, filling the template's variables
  from each contact's details (name, a saved field), so that every message is personal without a spreadsheet.
- As a practice manager, after a campaign finishes I send a follow-up only to people who didn't receive it, didn't
  read it, or read it but didn't reply, so that I reach people without spamming the rest.
- As a practice manager, I can trust that opted-out people and duplicates are left out, and I see how many were
  excluded and why, before anything is sent.

## Functional requirements

**Segments**
- R1 A segment is a named, saved set of rules over the organization's contacts, matched as **all** or **any** of:
  - has / doesn't have a tag;
  - a contact field (Weline's contact metadata) equals, doesn't equal, contains, is set, is not set;
  - belongs to a WhatsApp account (number);
  - last message **from** the contact within / not within N days (N ≤ 365);
  - contact added within / not within N days;
  - received / read / replied to / didn't read a given past campaign.
- R2 While editing, the live count of matching contacts and a sample of up to 20 (name and masked number) update as
  rules change. Opted-out and unreachable contacts are counted separately.
- R3 Segments can be renamed, edited, duplicated and deleted. A segment is re-evaluated each time it's used; sending
  takes a snapshot. **(assumption, Q3)**

**From segment to campaign**
- R4 "Create campaign" from a segment asks for a name, the WhatsApp account to send from, and an approved template.
  It creates a **draft** campaign in Weline's Campaigns with the segment's contacts as recipients, then links to it.
  Media headers, scheduling and starting are done in Campaigns as today.
- R5 Each template variable (header and body) is filled per recipient from a source: fixed text, the contact's name,
  the phone number, or a contact field, with a fallback when the field is empty. A preview shows the message for the
  first few recipients.
- R6 Before creating, a summary shows recipients and exclusions with reasons: opted out of marketing (for a marketing
  template), no usable number, duplicates (same number twice), and over the size limit. Nothing is created until the
  manager confirms. **(assumption: limit 10,000 recipients per campaign, Q5)**
- R7 Each contact appears at most once per campaign (by normalised number).

**Retargeting**
- R8 From a finished (or paused) campaign, build a follow-up from its recipients who: **didn't receive it** (failed or
  never delivered), **didn't read it** (delivered, not read), **read it but didn't reply** within N days, or
  **replied**. The follow-up is a new draft campaign (R4–R7), never a resend of the same campaign. **(Q4)**
- R9 A retarget can be saved as a segment rule ("didn't read campaign X") for reuse.

**History**
- R10 Every campaign created by Audiences is recorded: which segment or retarget, rules snapshot, counts and
  exclusions, who created it and when, and a link to the campaign.

**Access**
- R11 Permissions: `wl_audiences_segments` (read, create, update, delete) and `wl_audiences_sends` (create: turn a
  segment or retarget into a campaign). On install `admin` and `manager` get all; `agent` gets segments read.
  Creating a campaign also needs Weline's existing `campaigns:write`; starting it stays in Campaigns (`campaigns:execute`).
  **(assumption, Q1)**
- R12 Every screen shows loading, empty, error and forbidden states; read-only users see no edit controls.

**Consent**
- R13 Contacts opted out of marketing (Weline's `marketing_opt_out`, set by Meta's signal today and by STOP in step
  2.2) are never included for a marketing template, and are shown as excluded. Weline's campaign sender also skips
  them, as a second check.

## Non-functional

- Every query scoped to the active organization; tested with a second organization. Segment rules are compiled to
  parameterised SQL (no user text in SQL); contact field names are validated.
- A count over 50,000 contacts returns in under 1 second on staging.
- No phone numbers, names or message text in logs. The sample shows masked numbers.
- Strings under `wl.audiences.*`; keyboard-usable builder.
- Upstream campaign code is not changed: Audiences writes draft campaigns and recipients through upstream's models,
  exactly as Weline's own recipient import does.

## Out of scope (this slice)

- Segments based on appointments or visits (e.g. "no visit in 6 months"): needs the connector (Phase 3); recalls are
  Phase 5.
- Click tracking on template URL buttons, A/B tests, send-time optimisation.
- Opt-in capture and STOP handling (step 2.2), ad sources as segment rules (step 2.3).
- Recurring or automatic campaigns ("every Monday to new contacts").
- Cost estimates in currency (Meta's pricing varies by country and category); the summary shows counts only. **(Q6)**

## Open questions

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Who may build segments and create campaigns? | Admin and manager; reception can view segments only |
| Q2 | Which contact details do you want to segment on (tags, a "patient type" field, suburb, …)? | Tags, contact fields, activity, WhatsApp number, past campaigns |
| Q3 | Should a segment be a live definition (re-evaluated each time) or a frozen list? | Live definition; each campaign takes a snapshot |
| Q4 | Which follow-ups matter: not received, not read, read-no-reply, replied? | All four |
| Q5 | Maximum recipients per campaign? (Meta limits depend on your number's messaging tier.) | 10,000 |
| Q6 | Show an estimated Meta cost before sending? | No, counts only |
