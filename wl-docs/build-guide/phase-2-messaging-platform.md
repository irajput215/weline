# Phase 2: Messaging platform

**Goal:** close the WhatsApp-platform gaps that clinics (and later other businesses) will pay for: broadcasts to the
right patients, consent handled properly, ads that start conversations and can be measured, and one source of opening
hours. Background: [`../product/wati-comparison.md`](../product/wati-comparison.md); direction:
[ADR-002](../adr/ADR-002-connect-dont-build-pms.md).

**Prerequisites:** Phase 1 (1.1 and 1.2) merged.

[← Back to the guide](README.md)

Each step is a module (or a change to one) built with prompts A, B and C from [module-workflow.md](module-workflow.md),
unless it says otherwise. Upstream's campaigns, contacts and webhook code stay untouched: modules read and write
upstream data through upstream's models and services, and react to Weline events through the hooks receiver.

## 2.1 Audiences and retargeting

Module `audiences`. Add these to prompt B:

```text
Extra requirements for audiences:
- Segments: saved filters over the organization's contacts (tags, contact metadata fields, WhatsApp account,
  last message in/out within N days, opted in / not opted out). Live count before sending.
- "Send to segment": create an upstream campaign (draft) whose recipients are the segment's contacts, using
  upstream's campaign models (no changes to upstream campaign code). Template variables can come from contact
  fields. Opted-out contacts are always excluded.
- Retargeting: from a finished campaign, build a segment of recipients who didn't receive / read / reply, and
  start a follow-up campaign from it.
- Never send twice to the same contact for the same campaign; respect Meta's marketing template rules.
- Screens: Segments list and builder (with count), "Send campaign" from a segment, "Retarget" on campaign results.
```

## 2.2 Consent: opt-in and STOP

Module `consent`. Add to prompt B:

```text
Extra requirements for consent:
- Record opt-in with source and time (chatbot button or keyword, imported with proof, ad lead, web form) on the
  contact; never infer it.
- STOP, UNSUBSCRIBE, "stop messages" (case-insensitive, configurable list) on message.incoming through the hooks
  receiver: mark the contact opted out of marketing (upstream marketing_opt_out field plus a tag), reply once with a
  confirmation, and allow START to opt back in. Meta's own opt-out signal is respected too.
- Audiences (2.1) and every module that sends marketing messages exclude opted-out contacts. Service messages
  (appointment reminders the patient asked for) follow the clinic's policy, configurable.
- Check whether upstream's campaign sender skips marketing_opt_out contacts; if it doesn't, tell me before
  deciding how to handle campaigns created outside Audiences.
- Audit: every opt-in and opt-out change is recorded with time and source.
```

## 2.3 Click-to-WhatsApp ads attribution

Start with a spike, not a build:

```text
On branch spike/ctwa-referral from dev: find out whether Weline (upstream whatomate) keeps the "referral" object
Meta sends on the first message after a click-to-WhatsApp ad (ad id, headline, source URL, ctwa_clid). Check the
webhook handler, the stored message, and the message.incoming event payload our hooks receiver gets. Write the
findings and options in wl-docs/modules/ads/spike.md: (a) already available to modules, (b) a small upstream pull
request to whatomate to keep it, (c) anything else. Don't change upstream code; stop and show me.
```

Then module `ads` with prompts A, B, C once the route is agreed: tag the contact with the ad and campaign it came from,
show conversations and opt-ins per ad, and (after Phase 3) bookings per ad.

## 2.4 One source of opening hours

Small change, prompts B and C only:

```text
Weline has two sets of opening hours: upstream's chatbot business hours (used for the out-of-hours reply) and
Practice's location hours (used by built-in booking). Make upstream's business hours the single source for
"is the business open?" and the out-of-hours reply. Practice's location hours stay only for built-in booking slots,
and its screen says so and links to the chatbot business hours. Update wl-docs/modules/practice.
```

## WhatsApp templates (you, now)

Submit to Meta in Business Manager: appointment confirmation, 24-hour reminder, reschedule confirmation, cancellation,
recall, and one marketing template for a first broadcast test. Approval runs in parallel; Phase 5 needs the reminder
templates approved.

## Phase 2 gate

- [ ] A broadcast to a segment, and a retarget of non-readers, sent on staging without duplicates and without
      opted-out contacts
- [ ] STOP and START work end to end from a test phone
- [ ] The ads spike is decided (route agreed), even if the ads module waits
- [ ] Templates submitted to Meta

Next: [Phase 3: Connectors and Cliniko](phase-3-connectors-cliniko.md).
