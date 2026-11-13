# Reminders and recalls: design

Spec: [`requirements.md`](requirements.md). Code: `internal/modules/reminders/`, `frontend/src/modules/reminders/`.

## Shape

```text
           every 5 min (Redis lock wl:reminders:sweep)
 run loop ──────────────► Sweep ──► SweepOrg(org)
                                    │ rules (enabled reminders)
                                    │ provider.ListAppointments(now, now + max offset)   ← connector, live
                                    │ due? → claimSend (INSERT … ON CONFLICT DO NOTHING)
                                    └ sendTo: contactFor(GetCustomer mobile) → consent.Allowed → kit.Sender.SendTemplate

 hooks receiver ── message.incoming ──► Dispatcher: consent → reminders → … → ai
                                         reminders.HandleIncoming: replyKind → Reply → kit.Claim → SendText
```

## Data

- `wl.reminder_rules`: one table for both kinds (the guide's `wl.recall_rules` is the `kind = 'recall'` rows), with
  checks on kind, offset and recall-days ranges. `variables` and `service_ids` are JSONB arrays.
- `wl.reminder_sends`: `UNIQUE (organization_id, rule_id, target_id, start_at)`. For reminders `target_id` is the
  appointment id and `start_at` its start; a moved appointment has a new `start_at`, so it gets a new row (R4) while the
  old row stays as history. For recalls `target_id` is the customer id and `start_at` the first day of the month
  (R8). Status `pending → sent | failed | skipped` with a reason; `reply` and `replied_at` for R6. Foreign key to the
  rule with cascade (deleting a rule deletes its history; Q3).

Claiming the row before sending makes the sweep safe to run twice: a second instance (or a retry) loses the insert and
skips. A crash between claim and send leaves a `pending` row and no send: a miss rather than a duplicate, visible in
Activity.

## Contacts

Patients who never wrote to the clinic have no Weline contact. `contactFor` asks the connector for the customer
(`provider.GetCustomer`, new in this phase: built-in returns the contact's number, Cliniko the first mobile number,
normalised), finds the contact by number or creates one on the organization's default WhatsApp account. No mobile →
skipped `no_mobile`.

## Replies and claims

`kit.Dispatcher` now puts a claim slot in the context (`kit.Claim`, `kit.ClaimedBy`). Consent claims STOP/START,
Reminders claims reminder replies, and the AI agent ignores claimed messages, so a patient never gets two answers.
Reminders is registered before the AI agent. A reply only counts against an unanswered reminder for an appointment
that hasn't started (the 14-day maximum offset bounds its age), so "Confirm" in an unrelated chat goes to the agent.

## Connectors

`provider.Confirmer` is optional: built-in implements it; Cliniko has no confirm field in its public API, so Confirm
is recorded in Weline only (the gate checks this on the trial). Cancel goes through `Provider.Cancel` in both.

## Deviations from the build guide

- The sweep is a goroutine with a Redis lock rather than upstream's Redis queue/worker: upstream's worker only runs
  campaign jobs and adding a job type means editing upstream. Same effect: one sweep at a time across instances.
- The sweep lists appointments by start window rather than "changed since": a window read each sweep sees moves and
  cancellations without keeping a copy of the diary, and Cliniko filters it server-side.
- Reschedule hands over to a person (Q4).

## Limits

- Recall due lists read up to a year and a half of history from the connector on each open; fine for a clinic, slow
  for very large diaries (Cliniko is paged and rate-limited).
- The template must exist in the organization's default WhatsApp account.
