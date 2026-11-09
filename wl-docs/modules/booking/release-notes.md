# Booking: release notes

## 1.2: Appointments (first release)

Shipped in Weline Phase 1.2. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md). Builds on [Practice](../practice/release-notes.md).

### What's new

A **Bookings** section in the sidebar, a public booking page, and endpoints for WhatsApp chatbot flows.

- **Availability:** each practitioner's weekly hours per location (several blocks a day allowed), always clipped to
  the location's opening hours; time off (leave, meetings) that removes time without cancelling existing
  appointments (they're listed so staff can move them); online booking rules (minimum notice, how far ahead).
- **Appointments:** a day list with a practitioner filter; **New appointment** shows only free times for the
  service, location and practitioner (or "anyone"); find a patient by name or mobile, or add one; an optional
  administrative note. Reception can book outside hours with an explicit tick (recorded), but never on top of
  another booking. A patient can't hold two appointments at once unless staff say so.
- **Statuses:** Booked → Confirmed / Arrived / Cancelled / No-show; Confirmed → Arrived / Cancelled / No-show;
  Arrived → Completed. Completed, cancelled and no-show are final. Only booked and confirmed appointments can move.
  Cancelling needs a reason and frees the time. Every change and move is in the appointment's history with who made
  it.
- **No double booking, ever:** the app checks, and the database refuses an overlapping booking for the same
  practitioner even if two people book the same second.
- **Online booking:** `/book/<slug>` (the link from the Practice profile). Patients pick service, location,
  practitioner (or anyone), day and time, enter name and Australian mobile, accept the collection notice, pass a
  robot check, and get a 6-character reference. They're matched to an existing contact by mobile, or added as a new
  contact on the clinic's WhatsApp account. The **Online booking** page shows the link and what's still needed.
- **WhatsApp:** a chatbot flow can offer services and the next 10 free times as a list and book the one tapped, for
  the person messaging (by their number), and reply with a confirmation line.
- **Time zones:** everything follows the clinic's time zone, including daylight-saving days.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_booking_appointments` (view, book, move, change status, find/add patients) | read, create, update | read, create, update | read, create, update |
| `wl_booking_availability` (hours, time off, online rules) | read, update | read, update | read |

Appointments are never deleted; cancel them instead.

### Upgrade notes

- **Migration:** `booking/0001_create_booking` (five `wl.booking_*` tables, five permissions, and the Postgres
  `btree_gist` extension, a trusted extension the database owner can create). Additive; runs on deploy with
  `-migrate`. Check the deploy log for it.
- **Online booking needs captcha keys.** Create a Cloudflare Turnstile widget for the site's hostname and set
  `WELINE_TURNSTILE_SITE_KEY` and `WELINE_TURNSTILE_SECRET` in the host's environment (Render dashboard, never
  `render.yaml`). Until then the page loads but refuses bookings. Cloudflare's always-pass test keys are fine for
  staging only.
- **Rate limits behind a proxy:** keep `WHATOMATE_RATE_LIMIT__TRUST_PROXY=true` on Render so limits apply per
  visitor, not per proxy.
- **Organizations created after this deploy** need the `wl_booking_*` grants added in the role editor (as for
  Practice).
- **The old booking prototype is gone** (`/booking` page). Its unused table can be removed:
  `DROP TABLE public.booking_appointments;`

### For other modules

`internal/modules/booking` offers `SearchSlots`, `Create`, `Move`, `SetStatus`, `Get`, `List` and `History`, with
an `Actor` (staff user or source) so changes are attributed. Calendar (1.3) and Reminders (Phase 2) build on these.

### Known limitations

- No day or week calendar view yet (Calendar, 1.3).
- Web bookings send no WhatsApp confirmation (needs an approved template; Phase 2). Patients can't cancel or
  reschedule themselves yet.
- The public page and chatbot texts are English only.
- One opening block per day per location (from Practice); no buffers between appointments, recurring
  appointments, rooms, group sessions, payments or waitlists.
- No deletion job for old appointments yet (retention rule recorded; threat model open risk 11).
- Defaults for open questions Q1–Q10 are in use until confirmed.
