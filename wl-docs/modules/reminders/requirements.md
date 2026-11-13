# Reminders and recalls: requirements

**For:** the business owner or manager (clinic: practice manager), who wants fewer no-shows and patients coming back
on time, and reception, who see what was sent and pick up patients who want to move. Build guide Phase 5
([`../../build-guide/phase-5-reminders-recalls.md`](../../build-guide/phase-5-reminders-recalls.md)).

Status: **built on the defaults below; open questions still to confirm.**

## User stories

- As a patient, I get a WhatsApp reminder before my appointment (from the clinic's own system, built-in or Cliniko)
  and can reply Confirm, Cancel or Reschedule.
- As a patient who cancels from the reminder, the time is freed in the clinic's system straight away.
- As a patient who wants to move, a person from the team messages me to find a new time.
- As a patient who hasn't been back for a while, I get one recall message, unless I've opted out of marketing.
- As a practice manager, I set reminder rules (how long before, which approved template, which details fill it) and
  recall rules (how long after a visit), see what was sent and what patients replied, and send recalls from a due list.

## Functional requirements

- R1 **Rules** per organization (`wl.reminder_rules`): kind (`reminder` or `recall`), name, offset before the
  appointment (30 minutes to 14 days) or days after the last visit (7 to 1095), an **approved** WhatsApp template and
  its language, the variables that fill `{{1}}`, `{{2}}`, … in order (`name`, `service`, `when`, `location`,
  `practitioner`; recalls only `name` and `service`), optional services the rule applies to (empty = all), on/off.
- R2 **Upcoming appointments come from the connector** (`ListAppointments` for the next window), never from a copy of
  the diary. Works with the built-in diary and Cliniko.
- R3 **A sweep every 5 minutes**, one instance at a time (Redis lock): for each enabled reminder rule, each booked
  appointment whose reminder time has come and that hasn't started gets the template through `kit.Sender`.
- R4 **Never twice, never for the old time:** one send per (rule, appointment, start time), enforced by a unique key.
  A moved appointment is reminded again for its new time when that becomes due; cancelled or started ones never.
- R5 **Consent:** reminders are service messages and follow the consent module's rule for service messages after
  STOP; recalls are marketing and need marketing consent. Skipped sends are recorded with the reason.
- R6 **Replies** arriving on `message.incoming` that say Confirm, Cancel or Reschedule (button text or typed) apply to
  the contact's latest unanswered reminder for an appointment that hasn't started:
  - Confirm → confirmed in the connector where it supports it (built-in: status confirmed); thank-you reply.
  - Cancel → cancelled in the connector; confirmation reply. If the connector refuses, hand over to the team.
  - Reschedule → hand over to the team (agent transfer); "our team will message you" reply.
  The reply is claimed, so the AI agent and other modules leave it alone.
- R7 **Recall due list:** customers whose last visit for the rule's services was more than `recall_days` ago (looking
  back a year further), with nothing booked ahead, from the connector's appointment history.
- R8 **Sending recalls** is a person's decision: they pick customers from the due list and send. At most one recall per
  customer, rule and calendar month; customers no longer due are not sent.
- R9 **Permissions:** `wl_reminders_rules` read (admin, manager, agent), create/update/delete (admin, manager);
  `wl_reminders_recalls` read/create (admin, manager).
- R10 **Activity:** what was sent, skipped or failed and what the patient replied; no message text, no phone numbers.
- R11 **Audit:** rule changes and recall sends are written to the audit trail.

## Non-functional

- Every table has `organization_id`, every query filters by it.
- No phone numbers, names or message text in logs.
- Times stored in UTC; the template's date and time use the appointment's (or practice's) time zone.

## Open questions (built on the default shown)

- Q1 Should moving an appointment also send a "your appointment has moved" message? **Default: no; the reminder for
  the new time is the message.**
- Q2 Should recalls ever go automatically? **Default: no, a person sends them from the due list (recalls are
  marketing).**
- Q3 How long to keep the send history? **Default: kept until the rule is deleted; suggest one year.**
- Q4 Should Reschedule go to the AI agent's booking flow instead of a person? **Default: a person, until the AI agent
  is live with a real model.**
