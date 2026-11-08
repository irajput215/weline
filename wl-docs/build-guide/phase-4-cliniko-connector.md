# Phase 4: Cliniko connector

**Goal:** keep Weline in step with Cliniko, the practice management system the clinic already uses, behind a shared connector interface.

**Prerequisites:** the [Phase 3 gate](phase-3-ai-responses.md#phase-3-gate) is passed.

[← Back to the guide](README.md)

## Steps

Run prompts A, B and C from [module-workflow.md](module-workflow.md) for the `connectors` module.

Add to prompt B:

```text
Extra requirements for connectors:
- a Connector interface in internal/modules/connectors: ListPractitioners, ListServices, ListAppointments(since),
  CreateAppointment, CancelAppointment, FindOrCreatePatient
- wl.connector_accounts (encrypted credentials), wl.connector_links (entity, local id, provider, remote id),
  wl.connector_syncs (log)
- Cliniko implementation: HTTP Basic auth with the API key as username and empty password; the key's shard suffix
  picks the host (api.<shard>.cliniko.com/v1); send Accept: application/json and a User-Agent with our name and
  email; respect 200 requests/minute per user and back off on 429 using X-RateLimit-Reset
- sync by polling with updated_at filters on a schedule; first check https://docs.api.cliniko.com/ for whether
  webhooks now exist and tell me before choosing
- per-clinic mode: Cliniko is the source of truth OR built-in booking is, never both for one practitioner
- tests against recorded HTTP fixtures, no live calls in CI
```

## Phase 4 gate

- [ ] A parallel sync with a real Cliniko clinic shows no mismatches for two weeks

Next: [Phase 5: CRM and recalls](phase-5-crm-recalls.md).
