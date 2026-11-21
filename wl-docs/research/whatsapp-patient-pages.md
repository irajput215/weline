# Research: WhatsApp pages for receipts and medical documents

Date: 2026-11-18 · Status: desk research, no live tests yet

**Question:** can Weline send a patient a WhatsApp message that opens a page showing a receipt (billing) or a medical
document such as a transcript or letter?

**Answer:** yes, both are technically possible with what WhatsApp, Cliniko and Weline already offer. Receipts are
straightforward and are the first module ([`../modules/receipts/`](../modules/receipts/requirements.md)). Medical
documents use the same mechanism plus an identity check; whether to build them is a privacy decision, not a technical
one, and waits on a clinic's or a privacy adviser's view.

## Findings

| Question | Finding | Confidence |
| --- | --- | --- |
| Can a WhatsApp message open a web page? | Yes. An approved template can carry a URL button whose last part varies per recipient, so each patient gets their own link. Third-party guides say the link opens in the phone's browser and that taps send no webhook. | High |
| Does Weline support it? | Mostly. Upstream fills per-recipient URL button values (`ButtonURLParams` in `internal/handlers/messages.go`, `ButtonURLParamsToComponents` in `pkg/whatsapp/message.go`). `kit.Sender.SendTemplate` doesn't pass them yet: a small kit addition. Upstream also sends WhatsApp Flows (`SendFlowMessage`). | High (code read) |
| Receipts from Cliniko? | Yes. `GET /invoices`, `/invoices/{id}`, `/appointments/{id}/invoices`, items at `/invoices/{id}/invoice_items`. Fields include `number`, `issue_date`, `status` (10 Open, 20 Paid, 30 Closed, 40 Open credit), `total_amount`, `tax_amount`, `discounted_amount`, `net_amount` and **`online_payment_url`** (Cliniko's own pay page, nullable). **No PDF.** | High (Cliniko API reference) |
| Paying inside WhatsApp? | Not in Australia: WhatsApp's in-chat payments exist only in India, Brazil and partly Singapore, with limited access. Link to Cliniko's `online_payment_url` instead. | Medium-high |
| Clinical documents from Cliniko? | Treatment notes are readable (`draft`, `finalized_at`, structured `content`). Uploaded files (letters, reports) via `/patient_attachments` (category, filename, content link). No letters endpoint found. How long attachment download links last is unconfirmed. | High for endpoints |
| WhatsApp Flows (native screens)? | Can show server data, but need a 2048-bit RSA key pair registered with Meta and an encrypted endpoint. Built for short forms, not documents. Worth a look later for intake forms. | Medium (mostly third-party) |
| WhatsApp's policy on health? | Clinics are allowed. As reported by third parties, the policy says not to send or request health information where law requires handling that WhatsApp doesn't meet; health services in the US and France are refused. Australia isn't named. | Medium (Meta's page not reached) |
| Australian privacy law? | Health information is sensitive information; APP 11 requires reasonable steps to secure it. Nothing found from the OAIC that bans or approves messaging apps. Information about a health service provided to a person (including on an invoice) is health information. | Medium |

## What follows

- **Receipts:** template with a "View receipt" button → a page Weline serves, built live from Cliniko's invoice, with
  "Pay now" going to Cliniko's own pay page. Needs a module, secure links ([ADR-006](../adr/ADR-006-secure-patient-links.md)),
  a small `kit.Sender` addition and one approved template.
- **Medical documents:** same build plus an identity check (date of birth or a one-time code) before anything shows,
  finalised documents only, short expiry and an audit of every view. Clinical content never goes into the chat itself.
  Needs a decision (and ideally a clinic and a privacy adviser) before it's planned.
- **Not checked:** this session couldn't reach Meta's own sites (`developers.facebook.com`, `business.whatsapp.com`), so
  template button limits, Flows details and the policy wording come from third-party guides. Confirm them against
  Meta's documents before submitting the template (an item in the receipts [`dod.md`](../modules/receipts/dod.md)).

## Sources

- Cliniko API: [invoices](https://docs.api.cliniko.com/openapi/invoice),
  [treatment notes](https://docs.api.cliniko.com/openapi/treatment-note),
  [patient attachments](https://docs.api.cliniko.com/openapi/patient-attachment)
- Link buttons: [Bird](https://bird.com/docs/guides/whatsapp/message-types/interactive/cta-url-buttons),
  [LivePerson](https://developers.liveperson.com/whatsapp-business-messages-templates-interactive-cta-url-button-template.html)
- Flows: [Meta, encryption](https://developers.facebook.com/documentation/business-messaging/whatsapp/flows/guides/whatsapp-business-encryption/),
  [Meta, endpoint](https://developers.facebook.com/documentation/business-messaging/whatsapp/flows/guides/implementingyourflowendpoint),
  [Kapso](https://docs.kapso.ai/docs/whatsapp/flows/data-endpoint)
- Policy: [WhatsApp Business Messaging Policy](https://business.whatsapp.com/policy),
  [tyntec on health](https://www.tyntec.com/?p=35285)
- Payments: [Infobip](https://www.infobip.com/blog/whatsapp-payments),
  [360dialog](https://docs.360dialog.com/docs/messaging/payments.md)
- Privacy: [OAIC APP 11 guidelines](https://www.oaic.gov.au/__data/assets/pdf_file/0018/1287/app-guidelines-chapter-11-v1.2.pdf),
  [OAIC guide to information security](https://www.oaic.gov.au/privacy/privacy-resources/privacy-guides/guide-to-information-security)
