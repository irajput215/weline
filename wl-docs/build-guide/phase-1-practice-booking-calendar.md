# Phase 1: Practice, Booking, Calendar

**Goal:** a clinic can set up its practice, take bookings (by staff, the public booking page and WhatsApp) and run its day from a calendar.

**Prerequisites:** the [Phase 0 gate](phase-0-foundation.md#phase-0-gate) is passed.

[← Back to the guide](README.md)

Run A, B and C for each module, in this order. The prompts are in [module-workflow.md](module-workflow.md).

## 1.1 Practice

Clinic profile (name, time zone, booking slug), locations, practitioners linked to Weline users, services (name, duration, price, allowed roles), opening hours and closures. Settings screens. Every later module reads this.

## 1.2 Booking

Add these to prompt B:

```text
Extra requirements for booking:
- Prevent double booking twice: in the service, and with a Postgres exclusion constraint on practitioner_id and
  tstzrange(starts_at, ends_at) (needs the btree_gist extension; create it in the migration)
- Slot search only inside practice hours, practitioner availability and outside blocks, on the practice slot grid
- Status machine: BOOKED -> CONFIRMED/ARRIVED/CANCELLED/NO_SHOW; CONFIRMED -> ARRIVED/CANCELLED/NO_SHOW;
  ARRIVED -> COMPLETED; terminal states never change; every change written to wl.booking_status_history
- GET /api/x/booking/slots must be usable from a Weline chatbot api_call node (read features/chatbot.mdx for
  how api_call passes {{phone_number}} and maps the response)
- Public booking page at /book/<slug>: no login, rate limited, captcha, collection notice, returns a reference
```

Then ask for the WhatsApp flow:

```text
Write a step-by-step guide (wl-docs/modules/booking/chatbot-flow.md) for building the "Book an appointment"
chatbot flow in Weline's flow builder: ask service, api_call to /slots, show times as list buttons,
api_call to create the appointment, confirm. Include the exact api_call URLs, headers and response mappings.
```

## 1.3 Calendar

Day view (one column per practitioner, practice slot grid, now line, status colours), week view, drag to move or resize (calls Booking, which re-checks clashes), click to open the patient and their chat.

## Phase 1 gate

- [ ] A pilot clinic runs a full week of bookings in Weline
- [ ] WhatsApp templates submitted to Meta now (confirmation, 24-hour reminder, reschedule, cancellation, recall), because Phase 2 needs them approved

Next: [Phase 2: Reminders](phase-2-reminders.md).
