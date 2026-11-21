# Receipts: requirements

**For:** reception, who send a patient their receipt or invoice; patients, who open it from WhatsApp and pay if it's
still open. Build guide step 7.6 ([`../../build-guide/phase-7-connectors-verticals.md`](../../build-guide/phase-7-connectors-verticals.md)).
Background: [research note](../../research/whatsapp-patient-pages.md), [ADR-006](../../adr/ADR-006-secure-patient-links.md).

Status: **docs (prompt A), not built.** **Parked (2026-11-21):** a manual receipt is something owners can already send themselves; receipts return as an automatic step (sent when payment clears) once a connector with payment events exists, e.g. Square ([phase 8](../../build-guide/phase-8-appointment-smbs.md)). These docs are the starting point then; reshape R2 around an automatic trigger first.

## User stories

- As reception, I find a patient, see their invoices from the clinic's system (Cliniko), and send one to their
  WhatsApp with one tap, so they don't need an email address on file or a printed receipt.
- As a patient, I tap "View receipt" in the clinic's WhatsApp message and see what I was charged for, the total, GST and
  whether it's paid; if it isn't, I tap "Pay now" and pay on the clinic's own payment page.
- As reception, I see which receipts were sent, whether they were opened, and I can cancel a link sent to the wrong
  person.
- As a clinic, I'm confident Weline doesn't keep copies of invoices and that an old or cancelled link shows nothing.

## Functional requirements

- R1 **Invoices from the connected system.** For a contact, list their invoices from the organization's provider, newest
  first, the last 12 months: number, issue date, practitioner, total, status (open, paid, closed, credit). The patient is
  found with `provider.Finder` (never created in the clinic's system). A new optional provider ability
  (`provider.Invoicer`) supplies invoices; Cliniko has it. Without it the screen says the connected system doesn't share
  invoices.
- R2 **Send.** Staff send one invoice to the contact's WhatsApp as the approved receipt template (utility category) with
  a "View receipt" URL button carrying a new link (R4). The template's body names the clinic and the invoice number,
  nothing about the service. Consent is checked as a service message (`consent.PurposeService`); an opted-out contact
  is refused with the reason. A failed send is recorded and shown.
- R3 **Receipt page.** Opening the link shows, live from the connected system: clinic name, invoice number, issue date,
  practitioner, line items (name, quantity, amount), discount, GST, total, status, and a note that it's a copy of the
  clinic's invoice. If the invoice is open and the system gives a payment link, a "Pay now" button opens it. Amounts
  in the invoice's currency (AUD), dates in the practice's time zone. Mobile first, readable without JavaScript.
- R4 **Secure links** ([ADR-006](../../adr/ADR-006-secure-patient-links.md)): random token, only its hash stored; expiry
  30 days by default (Q1); revocable by staff; expired, revoked and unknown links show the same "expired" page; no
  caching, no referrer, not indexed; rate limited per IP.
- R5 **Sent list.** Per organization, newest first: contact (masked number), invoice number, sent by, sent at, status
  (sent, failed with reason, revoked, expired), first opened and number of opens. Filter by contact.
- R6 **Revoke.** Staff cancel a link; the page shows "expired" from then on. Recorded with who and when.
- R7 **Audit.** Send, revoke and each page view go to the audit log (`audit.Log`) without names, numbers, amounts or
  tokens.
- R8 **Connected system down or invoice gone:** the page says "can't show this receipt right now, try again later" (down)
  or "expired" (deleted or archived invoice); the staff list says the system is unavailable.
- R9 **Permissions:** `wl_receipts_sends` read, create and delete (revoke). Default roles: admin and manager all three;
  agent read and create (Q3).
- R10 **Isolation:** every query filters by organization; another organization's contact, invoice or send is "not
  found"; a link only ever shows an invoice of the organization that created it.

## Non-functional

- Weline stores no invoice contents: only the provider's invoice id, the invoice number, the token hash and send facts.
- Page under 50 KB, no external scripts or fonts; works on a 3G phone.
- Never log tokens, names, numbers or amounts (ADR-003's scrubber plus care in the module).
- Receipts aren't limited by the organization's plan (patients' service messages never stop, ADR-005).
- English only in this slice; page strings sit in one place in the module so a translation can follow.

## Questions (defaults used until you answer)

- Q1 Link expiry? **Default: 30 days**, a per-organization setting later if clinics ask.
- Q2 Should a receipt need a second check (date of birth) before showing? **Default: no** (same exposure as an
  emailed invoice); line items show the service name, which is health information, so revisit if a clinic objects.
- Q3 Can agents (front desk without a manager role) send receipts? **Default: yes, read and create; revoke is
  managers'.**
- Q4 Show the clinic's ABN on the page? **Default: only if Cliniko's invoice or business record supplies it** (to check in
  the Cliniko trial); the Cliniko invoice stays the official tax invoice.

## Out of scope (this slice)

- Sending automatically when an invoice is paid or created (Cliniko has no webhooks; needs polling: next slice).
- A PDF of the receipt (Cliniko's API has none; generating our own is a copy we'd then hold).
- Taking payments in Weline or WhatsApp (not available in Australia; Cliniko's pay page instead).
- Statements of several invoices, credit notes as their own documents, health fund or Medicare claim details.
- Providers other than Cliniko (the built-in diary has no invoices; others as their connectors grow `Invoicer`).
- Medical documents, letters or treatment notes (separate decision, see the research note and ADR-006 point 6).
- WhatsApp Flows.
- A "Send receipt" button inside upstream's chat screen (needs a seam; the timeline page links here instead).
