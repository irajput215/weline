# Phase 2: Reminders

**Goal:** automated confirmations and reminders around each appointment over WhatsApp, with Confirm, Reschedule and Cancel replies applied to the booking.

**Prerequisites:** the [Phase 1 gate](phase-1-practice-booking-calendar.md#phase-1-gate) is passed, including the WhatsApp templates submitted to Meta.

[← Back to the guide](README.md)

## Steps

Run prompts A, B and C from [module-workflow.md](module-workflow.md) for the `reminders` module.

Add to prompt B:

```text
Extra requirements for reminders:
- wl.reminder_rules (per practice offset, template, language) and wl.reminder_sends (unique key per appointment+rule)
- a job every 5 minutes using Weline's Redis queue/worker that sends due reminders through kit.Sender
- never remind cancelled appointments; never send twice; respect opt-in and opt-out ("STOP")
- reply buttons Confirm / Reschedule / Cancel handled through the webhook receiver (message.incoming)
  and applied to the appointment through the booking service
- tests with a fake clock for "due now", "already sent" and "cancelled"
```

## Phase 2 gate

- [ ] Templates approved by Meta
- [ ] On staging, no duplicate or missed reminders over several days

Next: [Phase 3: AI responses](phase-3-ai-responses.md).
