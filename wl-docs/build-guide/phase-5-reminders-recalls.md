# Phase 5: Reminders and recalls

**Goal:** confirmations and reminders for every appointment in the business's own system, with Confirm, Reschedule and
Cancel replies written back; recalls for customers who are due to come back.

**Prerequisites:** the [Phase 4 gate](phase-4-ai-agent.md#phase-4-gate) is passed and the reminder templates are
approved by Meta (submitted in Phase 2).

[← Back to the guide](README.md)

## Steps

Run prompts A, B and C from [module-workflow.md](module-workflow.md) for the `reminders` module.

Add to prompt B:

```text
Extra requirements for reminders:
- Upcoming appointments come from the connector (ListAppointments changed since), never from a copy of the diary.
- wl.reminder_rules (per organization: offset before the appointment, template, language, which services) and
  wl.reminder_sends (unique key per appointment + rule, status, message id).
- A job every 5 minutes using Weline's Redis queue/worker sends due reminders through kit.Sender.
- Never remind cancelled or moved appointments for the old time; never send twice; respect consent (Phase 2).
- Reply buttons Confirm / Reschedule / Cancel handled through the hooks receiver (message.incoming) and applied
  through the connector (Cliniko: confirm/cancel/reschedule there). Reschedule can hand over to the AI agent's
  booking flow (Phase 4).
- Recalls: wl.recall_rules (e.g. "no visit in 6 months for service X") evaluated against appointment history from
  the connector; recall messages use an approved template and respect consent; a due list on screen.
- Tests with a fake clock for "due now", "already sent", "cancelled" and "moved".
```

## Phase 5 gate

- [ ] On staging, several days of reminders with no duplicates or misses against the Cliniko trial
- [ ] Confirm, Reschedule and Cancel replies change the appointment in Cliniko
- [ ] A recall run sends only to customers who are due and haven't opted out

Next: [Phase 6: Multi-business SaaS](phase-6-saas.md).
