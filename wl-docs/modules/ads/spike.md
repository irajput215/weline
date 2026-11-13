# Ads: spike on click-to-WhatsApp referral data

Step 2.3 spike ([`../../build-guide/phase-2-messaging-platform.md`](../../build-guide/phase-2-messaging-platform.md#23-click-to-whatsapp-ads-attribution)),
2026-11-12, on `dev` at `d73fea3` (upstream base `047127c`; also checked `upstream/main` at `a78e635`).

**Question:** does Weline keep the `referral` object Meta sends on the first message after someone taps a
click-to-WhatsApp ad, so a module can tell which ad a contact came from?

**Answer: no.** Weline throws it away when it parses the webhook, so no module can see it today. Nothing in upstream
mentions `referral` or `ctwa` (neither our base nor the latest `upstream/main`).

## What Meta sends

On the first message from an ad (or a boosted post), the message object in the webhook carries:

```json
"referral": {
  "source_url": "https://fb.me/…",      // the ad or post
  "source_id": "120210000000000",       // ad id
  "source_type": "ad",                  // or "post"
  "headline": "Flu shots now open",
  "body": "Book in 30 seconds on WhatsApp",
  "media_type": "image",                // or "video"
  "image_url": "…", "video_url": "…", "thumbnail_url": "…",
  "ctwa_clid": "ARAkLk…",               // click id, needed later for Meta's Conversions API
  "welcome_message": { "text": "…" }
}
```

It comes once, on that first message, alongside the normal `text` (often the ad's prefilled message). Campaign and ad
set names aren't in it. Getting those from the ad id needs the Marketing API.

## Where it's lost (checked in code)

| Step | What happens to `referral` |
| --- | --- |
| Webhook parse: `internal/handlers/webhook.go` `WebhookHandler` decodes into `WebhookPayload`, whose messages are `IncomingTextMessage` (`internal/handlers/chatbot_processor.go`) | **Dropped.** The struct has no `Referral` field, and Go's JSON decoder silently ignores unknown fields. Shown by a throwaway test: the message parsed from a Meta-shaped payload with `referral` comes back without it |
| Stored message: `processIncomingMessage` → `models.Message` | Not stored. `messages.metadata` exists (jsonb) but nothing writes the referral into it, and the raw body isn't kept |
| Contact: `models.Contact` | Nothing about the source. `contacts.metadata` exists but isn't written from the webhook |
| Event to modules: `message.incoming` → `handlers.MessageEventData` (`internal/handlers/webhook_dispatch.go`) → our hooks receiver | No referral field. Modules get the message id, contact, type and text only |
| `pkg/whatsapp` `WebhookMessage` (the library's own types) | Also no referral field |

## Options

### (a) Already available to modules

**Not available.** The only ad-related thing a module sees today is the message text. If the ad has a prefilled
message, the patient's first message is usually that text, unless they edit it.

### (b) A small upstream pull request to whatomate (recommended)

A change to upstream, sent to whatomate rather than patched in our fork (the seam rules forbid that):

1. Add `Referral *Referral` to `IncomingTextMessage`, with the fields above.
2. In `processIncomingMessage`, when it's present, save it to `messages.metadata["referral"]` (the column already
   exists, so no migration).
3. Add a `Referral` field (JSON `referral`, omitted when empty) to `MessageEventData` so `message.incoming` webhooks carry it.
   This helps every whatomate user who sends webhooks to their own CRM, not just Weline.
4. Optionally show "From ad: <headline>" in the chat.

It's about 40 lines plus a test, and generally useful to whatomate (ad attribution is a common ask for WhatsApp
inboxes), so it has a fair chance of being accepted. Our `ads` module would then read `e.Data` (step 3) or
`messages.metadata` by `message_id` (step 2). **Cost:** we wait for upstream review and a release, and we take it
with the next upstream sync, which is currently blocked by the Oat UI change (see Consent `dod.md`). Until then
only (c1) works.

### (c) Anything else

**c1. Ad codes in the prefilled message (works today, no upstream change).** Each ad's prefilled message carries a
short code, for example "Hi, I'd like to book a flu shot [FLU24]". The `ads` module listens to `message.incoming`
(like Consent), matches a code it knows on a contact's first messages, and tags the contact with the ad and campaign.
- The same codes work for wa.me links on the website, Google ads, posters and QR codes, which never get a Meta
  referral at all.
- **Limits:** a patient can delete the code. There's no ad id or `ctwa_clid` (so no Conversions API later), and the
  codes have to be typed into Meta Ads Manager by hand.

**c2. Weline receives Meta's webhook first and passes it on (no upstream change, more risk).** A module route outside
`/api` (for example `/x/ads/meta-webhook`) set as the callback URL in the Meta app. Meta also allows overriding it
per WhatsApp Business Account or phone number. The route records any `referral`, then hands the same request to
upstream's `WebhookHandler` in-process.
- **Against:**
  - every incoming WhatsApp message would depend on module code;
  - we'd re-check Meta's signature ourselves;
  - the hand-over relies on upstream's handler signature staying the same;
  - it changes setup for every business.
- Keep it as a fallback if upstream won't take (b).

**c3. Meta's Marketing API (reporting only).** The ads' insights give "messaging conversations started" per ad, but
not which contact. It's useful later for cost per booking next to our own counts, and needs Marketing API access
per business. Not attribution on its own.

## Recommendation

1. **Send (b) upstream now.** It's the only way to get the real ad id and click id. You'd open it, or tell me to
   prepare it on a branch of a whatomate fork. It needs you because it goes to another repository.
2. **Build the `ads` module on c1 first** (prompts A, B, C):
   - ad codes with their campaign, matched on `message.incoming`, tagging the contact with the ad and campaign;
   - conversations and opt-ins per ad (with Consent's `ad` source, already reserved);
   - bookings per ad after Phase 3.

   Design the source so that once (b) lands, Meta's referral fills the same record with source `meta_referral`, the
   ad id and `ctwa_clid`, and wins over a code.
3. Keep **c2** as the fallback if (b) is refused or stalls for more than one upstream release.

## Decision (2026-11-13)

- **No upstream pull request** (option b is not pursued). Meta's ad id and `ctwa_clid` stay out of reach for now.
- **Which entry points clinics use is not known.** So the module goes with **c1, ad codes**, which works the same for
  Meta ads, the website, Google, posters and QR codes.
- c2 (receiving Meta's webhook first) stays a fallback, to reconsider only if a clinic needs the real ad id or
  Meta's Conversions API.
- Next: module `ads`, prompts A, B, C, on c1. Specs in `requirements.md` and `design.md` next to this file.
