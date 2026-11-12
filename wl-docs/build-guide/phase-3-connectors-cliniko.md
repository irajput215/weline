# Phase 3: Connectors and Cliniko

**Goal:** Weline reads free times from, and books into, the system the business already uses, starting with Cliniko,
through one connector interface that the WhatsApp booking flow, the AI agent and reminders all use.
Direction: [ADR-002](../adr/ADR-002-connect-dont-build-pms.md).

**Prerequisites:** the [Phase 2 gate](phase-2-messaging-platform.md#phase-2-gate) is passed.

[← Back to the guide](README.md)

## 3.0 Cliniko access and API spike

**You first:**
1. Sign up for a Cliniko free trial (never develop against a real clinic's data). Create a user "Weline integration",
   then an API key for it (My Info → API Keys). Add it to the cloud environment as a secret when Claude asks; never
   paste it in chat.
2. Contact Cliniko: API terms for a commercial product used by many clinics, rate limits, whether there is a partner
   listing, and whether webhooks or OAuth are offered to partners.

Then:

```text
On branch spike/cliniko from dev: using https://docs.api.cliniko.com/ and the trial account's API key, check that
Cliniko supports every ability Weline needs and record the exact endpoints, fields and limits in
wl-docs/modules/connectors/cliniko-spike.md:
- list businesses (locations), practitioners, appointment types (services) and their durations
- find available times for an appointment type and practitioner over a date range
- find a patient by mobile, create a patient
- create, reschedule and cancel an appointment; read appointments changed since a time
- webhooks (do they exist now?), rate limits and headers, the User-Agent requirement, the shard in the API key
Make real calls against the trial only. Don't build the connector; stop and show me what's possible and what isn't.
```

## 3.1 Connector layer

Module `connectors`, prompts A, B, C. Add to prompt B:

```text
Extra requirements for connectors:
- One interface of abilities, used by every caller: ListLocations, ListPractitioners, ListServices,
  FindFreeTimes(service, location, practitioner?, range), FindOrCreateCustomer(name, mobile),
  Book, Reschedule, Cancel, ListAppointments(changed since).
- wl.connector_accounts (provider, encrypted credentials with Weline's crypto, status, last sync),
  wl.connector_links (entity, local id, provider, remote id), wl.connector_syncs (log).
- One active provider per organization (or per location if the spike shows it's needed). Booking (1.2) becomes the
  "built-in" provider behind this interface, unchanged in behaviour and tests.
- Booking's chatbot endpoints (catalog, slots, book) call the active provider, so the WhatsApp flow from
  wl-docs/modules/booking/chatbot-flow.md works with any provider without changes to the flow.
- Screens: Settings → Connected system (choose provider, enter credentials, test connection, last sync, errors).
```

## 3.2 Cliniko connector

Prompts A, B, C for the Cliniko provider in module `connectors`. Add to prompt B:

```text
Extra requirements for the Cliniko provider:
- Implements every ability from 3.1 using the endpoints recorded in cliniko-spike.md.
- HTTP Basic auth with the clinic's API key as username and empty password; the key's shard suffix picks the host;
  Accept: application/json and a User-Agent with our name and contact email; respect the rate limit and back off on
  429 using the reset header.
- Changes: webhooks if the spike found them, otherwise polling with updated_at filters on a schedule.
- Cliniko is the source of truth: Weline never keeps its own copy of the diary, only ids in wl.connector_links and a
  short cache of free times.
- Tests against recorded HTTP fixtures from the trial account; no live calls in CI.
```

## 3.3 Privacy before the first real clinic

A Cliniko key opens a clinic's patient records, so these move ahead of any real clinic connecting. Plan each with
prompt A (or an ADR), then build:

- **Australian hosting:** move staging and production to an Australian region (Render's free tier is in Oregon and
  its free Postgres expires on 8 December 2026). You choose the host; Claude writes the migration plan.
- **Audit trail** for module changes (connector settings, bookings, consent changes) and connector credential access.
- **Patient identifiers out of logs:** upstream logs phone numbers and names on every message. Decide the route
  (log level in production, a scrubbing log handler, or an upstream pull request); don't edit upstream code without
  that decision.
- **Retention and deletion:** jobs for what Weline stores (messages per Weline's policy, booking records for the
  built-in provider, sync logs).

## Phase 3 gate

- [ ] A pilot clinic on Cliniko books through WhatsApp for two weeks with no mismatches between Weline and Cliniko
- [ ] The 3.3 privacy items are done before that clinic's real data flows

Next: [Phase 4: AI agent](phase-4-ai-agent.md).
