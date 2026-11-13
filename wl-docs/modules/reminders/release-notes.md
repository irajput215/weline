# Reminders and recalls: release notes

## Phase 5: reminders and recalls (first release)

Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md). API: [`api.md`](api.md).

### What's new

**Reminders → Reminders & recalls**, with three tabs:

- **Rules:** add a reminder (for example, 24 hours before every appointment) or a recall (for example, 180 days after
  a visit), choose an approved WhatsApp template and what fills its blanks (first name, service, date and time,
  location, practitioner). Turn rules on and off.
- **Activity:** every reminder and recall sent, skipped (and why: opted out, no mobile number…) or failed, and what the
  patient replied.
- **Recalls due:** who is due back for each recall rule; tick patients and send.

**What patients get on WhatsApp:**

- A reminder from your template before each appointment in your system (built-in or Cliniko). Moved appointments are
  reminded for the new time; cancelled ones never; nobody gets the same reminder twice.
- Replying **Confirm** confirms it; **Cancel** cancels it in your system and frees the time; **Reschedule** passes the
  chat to your team.
- At most one recall a month per rule, and only with marketing consent.

### Before you turn it on

- Get the reminder and recall templates approved by Meta (Templates). Use buttons Confirm, Cancel and Reschedule.
- Check the consent settings: whether reminders still go to patients who replied STOP.
- With Cliniko, Confirm is recorded in Weline (Cliniko's API has no confirmation field).
