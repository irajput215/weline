# Calendar: test plan

Cases that must exist before the module is done. Go tests run against a real Postgres and Redis; handler tests also
run as a user of a **second organization**. Fixtures reuse the Practice and Booking services.

## Service and handlers (Go)

| Case | Covers |
| --- | --- |
| Day view: columns are active practitioners at the location, by name; inactive or other-location practitioners left out; practitioner filter narrows | R1 |
| Range = opening hours, stretched by an appointment or working block outside them; closed day (closure) flagged with reason | R2 |
| Working ranges = availability ∩ opening; blocks clipped to the day with reason; nothing working on a day off | R3 |
| Appointments placed in the right column with local start/end minutes; cancelled hidden unless `show_cancelled`; another location's appointment for a shown practitioner marked `elsewhere` | R4 |
| Week view for one practitioner: seven columns Mon–Sun; `all`: counts per day and practitioner | R6 |
| Sydney October and April changes: 23- and 25-hour days place cards at the right local minutes; Brisbane unaffected | non-functional |
| `is_me` set for the practitioner linked to the caller | R14 |
| Route needs `wl_calendar_view` (403 without, 401 without a user); reception (agent) allowed by default | R13 |
| Second organization: its location or practitioner ids return 404; the first org's data never appears | non-functional, threats |
| Missing location with several locations → 400; unknown view or bad date → 400; practice not set up → 409 | api.md |

**Booking change (R8)**

| Case | Covers |
| --- | --- |
| Resize longer and shorter in whole slots; off-grid, zero-length, over 8 hours refused | R8 |
| Resize into another appointment refused (409 names it); into time off or outside hours refused unless `outside_hours` | R8, R9 |
| Move keeps duration when only `starts_at` changes; history records `ends_at` old → new on resize | R7, R8 |
| Only BOOKED/CONFIRMED can be resized | R7 |

## Frontend (Vitest)

| Case | Covers |
| --- | --- |
| Grid maths: minute ↔ pixel, snapping to the slot grid, overlapping cards laid out side by side | R4, R7 |
| Drag preview computes new start, column and end; drop calls Booking's PUT with the right body; 409 reverts and shows the message; 400 "isn't available" asks to book outside hours and retries with `outside_hours` | R7–R9 |
| Now line position for today, hidden on other days | R5 |
| Store: load, empty, error, 403 → forbidden; refresh timer pauses while dragging and when the tab is hidden | R12, R15 |
| Read-only user: no drag handles, no slot booking, no actions | R13 |
| Routes and nav carry `wl_calendar_*` permissions (module index convention test) | R13 |

## End to end (Playwright, one journey per user story)

1. Reception opens today's calendar for a location: one column per practitioner, cards with status colours, the now
   line, shaded non-working time and a time-off block with its reason.
2. Reception drags a card to a free time in another practitioner's column; it moves and the history shows it. Dragging
   onto another appointment snaps back with the clash message.
3. Reception resizes a card to two slots; the next slot is no longer offered in New appointment.
4. Reception clicks a card: the panel shows details and actions; marking Arrived updates the card colour; the chat
   link opens `/chat/<contact>`.
5. Reception clicks an empty slot and books it through the New appointment dialog prefilled with that practitioner and
   time.
6. A practitioner linked to a login sees their column first and can switch on "Just me".
7. A manager opens the week for one practitioner and moves an appointment to another day.

Journeys live in `frontend/src/modules/calendar/e2e/` and run in the `module-e2e` CI job.
