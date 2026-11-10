# Calendar: requirements

**For:** reception (runs the day from it), practitioners (see their own day), and practice managers (see the week).

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without pilot-clinic input; each one is
listed under [Open questions](#open-questions) for you to confirm or correct before step B (build).

Builds on Practice (1.1: clinic time zone, slot grid, locations, opening hours, closures) and Booking (1.2:
appointments, availability, time off, the Go service that applies every booking rule). Calendar stores nothing new
about patients; it is a view of Booking with drag-to-change.

## User stories

- As a receptionist, I open today's calendar for my location and see one column per practitioner on the clinic's
  slot grid, with appointments coloured by status and a line showing the time now, so that I can run the day at a
  glance.
- As a receptionist, I drag an appointment to another time or another practitioner's column, or drag its bottom edge to
  make it longer or shorter, and Weline refuses the change if it clashes, so that rescheduling takes seconds and stays
  safe.
- As a receptionist, I click an appointment to see its details and status actions, and jump to the patient or their
  WhatsApp chat, so that I can confirm or follow up without searching.
- As a receptionist, I click an empty slot to book it, with the practitioner and time already filled in.
- As a practitioner with a Weline login, I see my own column by default, so that I know my day.
- As a practice manager, I see the week for one practitioner or the whole location, so that I can spot gaps and
  overbooking.

## Functional requirements

**Day view**
- R1 For a chosen date and location, one column per active practitioner who works at that location, ordered by
  display name. A practitioner filter can narrow it to one or a few. **(assumption: columns are practitioners at one
  location at a time, Q2)**
- R2 Rows follow the clinic's slot grid in its time zone, from the location's opening time to its closing time that
  day, extended if an appointment or working block falls outside them. Closed days say so (closure reason or "closed").
- R3 Each column shades time the practitioner doesn't work (outside their availability) and shows time off (blocks)
  with its reason, so free time is obvious.
- R4 Appointments appear as cards spanning their duration, showing time, patient name, service and a status colour;
  cancelled appointments are hidden by default and can be shown. Overlapping cards in one column (only possible for
  staff overrides) sit side by side.
- R5 A "now" line marks the current time on today's view and moves as time passes.

**Week view**
- R6 For a chosen week (Monday to Sunday) and one practitioner, seven day columns with the same grid, shading and
  cards. For "all practitioners" the week shows a compact count per day and practitioner instead of cards.
  **(assumption, Q4)**

**Changing appointments**
- R7 Dragging a BOOKED or CONFIRMED appointment to another time, day (week view) or practitioner (day view) moves it
  through Booking, which re-checks every rule (grid, hours, availability, time off, clashes, the patient's other
  appointments) and records the move in the appointment history. Refused moves snap back and show Booking's message.
- R8 Dragging the bottom edge resizes an appointment in whole slots (at least one slot, at most 8 hours), through
  Booking with the same checks and history. This needs Booking to accept a new end time from staff; that change is
  part of this step. **(assumption: resizing is allowed for staff, Q3)**
- R9 Moves and resizes outside a practitioner's hours ask for confirmation and are then made as "outside hours", as in
  Booking. Clashes are never allowed.
- R10 Clicking a card opens a side panel with the appointment details, its allowed status actions (same rules as the
  Appointments page), its history, and links to the patient's contact page and their WhatsApp chat (each shown only to
  users with the matching Weline permission).
- R11 Clicking an empty slot opens Booking's New appointment dialog with that practitioner, date and time chosen.

**Keeping current**
- R12 The calendar refreshes on its own every 30 seconds and after every change, so several reception screens stay in
  step. **(assumption: polling, not live push, Q5)**

**Access**
- R13 Permission `wl_calendar_view` (read). On install `admin`, `manager` and `agent` get it. Moving, resizing and
  status changes also need `wl_booking_appointments` update; booking from a slot needs create. Users without them see a
  read-only calendar.
- R14 A practitioner whose Weline login is linked (Practice) sees their own column first, with a "Just me" filter that
  is remembered on that browser. **(assumption, Q6)**
- R15 Every screen shows loading, empty, error and forbidden states.

**For other modules**
- R16 One read endpoint returns everything a view needs (practitioners, working time, time off, closures, appointments)
  for a day or week, so the browser makes one request per refresh rather than one per practitioner.

## Non-functional

- Every query is scoped to the active organization (tested with a second organization). Calendar adds no tables and
  no patient data of its own; it shows patient names only to users who can already see appointments.
- Times come from Booking as UTC instants and are drawn in the clinic's time zone; tested on Sydney's daylight-saving
  days (a day with 23 or 25 hours).
- A day with 10 practitioners and 150 appointments loads in under 500 ms on staging and drags smoothly on a mid-range
  laptop. The grid is usable with a keyboard: cards are focusable, Enter opens the panel, and moves can also be made
  from the panel's time picker, so drag is never the only way.
- No new frontend libraries: the calendar grid is built with Weline's existing components (a new npm dependency
  would change upstream's `package.json`).
- Strings under `wl.calendar.*`.

## Out of scope (this slice)

- Saved views and per-user layout preferences (`wl.calendar_views`).
- Month view, rooms or resources as columns, printing the day list.
- Live push updates (websockets), drag between locations in one view.
- Google Calendar / Outlook sync for practitioners (Connectors, Phase 4).
- Editing practitioner availability or time off on the calendar (Bookings → Availability does that).

## Open questions

Defaults above are used until you answer. Please confirm or correct:

| # | Question | Default used |
| --- | --- | --- |
| Q1 | Should the calendar replace the Appointments list as reception's main page, or sit beside it? | Beside it: a new "Calendar" page, the list stays |
| Q2 | Columns: practitioners at one location, or rooms, or all locations at once? | Practitioners, one location at a time |
| Q3 | May reception change an appointment's length (drag to resize), or is length always the service's duration? | Yes, in whole slots, staff only; recorded in history |
| Q4 | Week view: one practitioner's week, or the whole location's week? | One practitioner in detail; "all" as counts per day |
| Q5 | Is refreshing every 30 seconds enough, or must other screens update instantly? | 30 seconds plus after every change |
| Q6 | Should practitioners see only their own column, or everyone's? | Everyone's, own column first; a filter for "just me" |
| Q7 | Day range: opening hours only, or a fixed range like 07:00–20:00? | Opening hours, stretched to fit anything outside them |
