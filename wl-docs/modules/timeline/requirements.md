# Contact timeline: requirements

**For:** reception, managers and admins who want to see, in one place, what has happened with a contact before
replying or calling them. Build guide step 7.5 ([`../../build-guide/phase-7-connectors-verticals.md`](../../build-guide/phase-7-connectors-verticals.md)).

Status: **built.**

## User stories

- As reception, I search a contact by name or part of their number and see their history newest first: messages,
  broadcasts, appointments, reminders and replies, consent changes, the ad or link they came from, what the AI agent
  did and when they were passed to the team.
- As a clinic, I'm confident the timeline shows administrative facts only, never message contents, notes or anything
  clinical.

## Functional requirements

- R1 **Search** by name or number (at least 2 characters, numbers matched by digits, Australian 04… numbers matched as
  614…); at most 20 results; numbers masked (first 2 and last 3 digits).
- R2 **Timeline** for one contact, newest first, 100 items a page with "show older": messages (type and status), broadcasts
  (campaign and status), appointments from the connected system (service, practitioner, location, status; a year back
  and a year ahead), reminders (rule, status, reason, reply), consent (opt in/out, source), ad arrivals (entry point,
  channel), AI agent runs (intent, outcome) and handovers (status, source).
- R3 **Administrative only:** no message text, consent evidence, handover notes or clinical information; the number is
  masked; message contents stay in Chat (linked).
- R4 **Never creates anything:** looking a contact up in the connected system uses a find-only lookup (`provider.Finder`);
  Cliniko never gets a new patient from a timeline view.
- R5 **Connected system down:** the rest of the timeline still shows, with a note.
- R6 **Permissions:** `wl_timeline_contacts` read for admin, manager and agent (the people who answer chats).
- R7 **Isolation:** another organization's contact is "not found"; every query filters by organization.

## Not in this step

- A timeline panel inside upstream's chat screen (needs an upstream seam; the page links to Chat instead).
