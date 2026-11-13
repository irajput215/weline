# Ads: requirements

**For:** the business owner or manager who pays for ads and promotions (clinic: practice manager) and wants to know
which ones bring patients. Build guide step 2.3
([`../../build-guide/phase-2-messaging-platform.md`](../../build-guide/phase-2-messaging-platform.md#23-click-to-whatsapp-ads-attribution)),
route chosen in the [spike](spike.md#decision-2026-11-13): **ad codes in the pre-filled message**, no upstream change.
Follows [Consent](../consent/requirements.md) and [Audiences](../audiences/requirements.md).

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without customer input; each one is
listed under [Open questions](#open-questions).

Clinics pay for Facebook and Instagram ads, Google ads, flyers and a "Message us on WhatsApp" button on their website,
but every chat lands in the same inbox and nobody knows which one worked. Meta's own ad data never reaches Weline (see
the spike), so this module gives every place a patient can start a chat its own **entry point**: a WhatsApp link with
a short **code** in the pre-filled message. When a message with the code arrives, Weline records where the patient
came from, tags the contact, and counts conversations, new contacts, opt-ins and bookings per entry point and
campaign.

## User stories

- As a practice manager, I create an entry point for each ad, flyer or website button, get a WhatsApp link to paste
  into it, and don't have to type codes by hand.
- As a practice manager, I see for each entry point and campaign how many people started a chat, how many were new,
  how many later agreed to marketing and how many booked, so that I know where to spend.
- As reception, I see on the contact where a patient came from (a tag), so that I can greet them about the offer
  they saw.
- As a practice manager, I can build an Audiences segment of everyone who came from a campaign (through the tag).

## Functional requirements

**Entry points**
- R1 An **entry point** has:
  - a name (for example "Flu shot ad, Instagram, May");
  - a **code**: 3 to 12 letters and digits with at least one of each, so it can't be an ordinary word like STOP or
    HELLO. It's unique in the organization and never reused, even after archiving. The form suggests one from the
    name, for example `FLU24`;
  - a **channel**: Meta ad, Google ad, website, QR code or flyer, email, other;
  - an optional **campaign** label that groups entry points (for example "Flu season 2026"), picked from the
    organization's existing labels or typed;
  - the **WhatsApp number** the link opens (digits with country code; the form remembers the last one used);
  - the **pre-filled message** (default "Hi! I'd like to know more [CODE]"), which must contain the code;
  - a **tag** added to contacts who arrive (default `from-<code>`, lower case), and whether to also add the campaign
    as a tag (default on, `campaign-<label>`, slugified). **(assumption, Q2)**
- R2 For each entry point Weline shows the **WhatsApp link** (`https://wa.me/<number>?text=<message>`) with a copy
  button, and the exact text to paste as the pre-filled message in Meta Ads Manager.
- R3 Entry points can be edited (name, channel, campaign, number, message, tag), but not their code. They're
  **archived**, not deleted, once anyone has arrived through them; archived codes stop matching. An entry point
  nobody arrived through can be deleted.

**Arrivals**
- R4 An incoming WhatsApp **text** message that contains an active code as a whole word records an **arrival** for
  the contact. Matching ignores case. The code may stand alone or in brackets (`FLU24`, `[FLU24]`, `#FLU24`). Messages
  longer than 1,000 characters are skipped. If several codes appear, the first one wins.
- R5 One arrival per contact per entry point per 24 hours: sending the code again the same day doesn't count twice.
  The same message processed twice records one arrival.
- R6 An arrival records:
  - the entry point;
  - the contact and the message it came from;
  - when it happened;
  - whether the contact was **new**, meaning their first message to the organization was this one or came at most
    10 minutes before it. **(assumption, Q3)**
- R7 On an arrival, the contact gets the entry point's tag (and the campaign tag when set), through upstream's
  contact model. Tags are never removed by this module.
- R8 An arrival is **not** consent to marketing (Consent R8). Opt-ins still come only from Consent.

**Reports**
- R9 The **Entry points** page lists every entry point with, for a chosen period (default the last 30 days):
  - conversations (arrivals);
  - new contacts;
  - **opted in**: contacts whose latest arrival before the opt-in was through this entry point, and who opted in
    within 30 days of it;
  - **booked**: the same rule for appointments created in Weline's built-in Booking within 30 days (counted only when
    the Booking module is installed; Cliniko bookings come with Phase 3).

  **(assumption, Q4)** Totals per campaign are shown too.
- R10 An entry point's page shows its link, settings and recent arrivals (when, contact name and masked number, new
  or returning, opted in, booked), newest first, 50 per page.
- R11 Permissions:
  - `wl_ads_entrypoints`: read (list, reports, arrivals), create, update, delete (archive or delete).

  On install, `admin` and `manager` get all; `agent` (reception) gets none, because the contact tag already shows
  them where a patient came from. **(assumption, Q5)**
- R12 Every screen shows loading, empty, error and forbidden states; read-only users see no edit controls.

## Non-functional

- Event handling is quick and idempotent: one cached read of the organization's active codes, at most one
  transaction, no outgoing messages.
- Every query is scoped to the organization and tested with a second organization.
- No phone numbers, names or message text in logs. The arrival keeps the matched code, never the message.
- Counts for 200 entry points and 50,000 arrivals return in under 1 second.
- Strings under `wl.ads.*`. No upstream code changed; contacts' tags updated through upstream's model.

## Out of scope (this slice)

- Meta's own ad data (ad id, headline, `ctwa_clid`): not available without an upstream change (spike option b,
  declined). If it ever is, it would fill the same arrival record with its own source.
- QR code images: Weline has no QR library, and adding one changes upstream's `package.json`. Paste the link into
  any QR generator for now.
- Ad spend, cost per conversation or booking (needs Meta and Google reporting APIs), and Meta's Conversions API.
- Bookings made in Cliniko (Phase 3) and revenue.
- Removing the code from the message the inbox shows, and short links or UTM tracking for websites.
- An Audiences rule "came from entry point or campaign" (the tags cover it for now).
- Using an arrival as a Consent opt-in source (`ad` stays reserved).

## Open questions

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Which entry points do clinics use: Meta ads, Google, website, QR at the desk, flyers? | Unknown (2026-11-13); all supported the same way |
| Q2 | Should arriving contacts get tags, and named how? | Yes: `from-<code>` and `campaign-<label>`, both editable per entry point |
| Q3 | When does a contact count as "new"? | Their first message to the organization was the code message or at most 10 minutes before it |
| Q4 | How long after arriving does an opt-in or booking still count for the entry point? | 30 days, credited to the latest arrival before it |
| Q5 | Should reception see the Entry points pages? | No; they see the tag on the contact |
