# Booking: requirements

**For:** reception (books, moves and cancels appointments, marks arrivals), the practice manager (sets when each
practitioner works), patients (book themselves online or over WhatsApp), and later modules (Calendar, Reminders,
AI, Connectors), which read and change appointments through this module.

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without pilot-clinic input; each
one is listed under [Open questions](#open-questions) for you to confirm or correct before step B (build).

Builds on Practice (1.1): clinic profile, time zone, slot grid, locations, practitioners, services, opening hours
and closures all come from there.

## User stories

- As a practice manager, I set the weekly hours each practitioner works at each location, and add one-off blocks
  (leave, meetings), so that only real free time is offered.
- As a receptionist on the phone, I pick a service, location and (optionally) a practitioner, see the free times,
  and book one for an existing or new patient, so that booking takes seconds and can't clash.
- As a receptionist, I move or cancel an appointment and mark patients arrived, completed or no-show, so that the
  day's list is accurate, and every change is recorded with who made it.
- As a patient, I open the clinic's booking link (`/book/<slug>`), pick a service, place and time, enter my name
  and mobile, and get a booking reference, without creating an account.
- As a patient on WhatsApp, I message the clinic, choose a service and one of the offered times from a list, and
  get a confirmation in the same chat.
- As another Weline module (Calendar, Reminders, AI), I list, create, move and change the status of appointments
  through a Go service that applies the same rules as the screens.

## Functional requirements

**Practitioner availability**
- R1 Each practitioner has weekly working blocks per location: weekday, start and end on the clinic's slot grid,
  several blocks per day allowed (e.g. 08:00–12:00 and 13:00–17:00). A practitioner with no blocks is not
  bookable. **(assumption: availability is set per practitioner, not inherited from opening hours, Q2)**
- R2 A block (leave, meeting, training) removes a practitioner's time between two instants, with a reason. Blocks
  can be added and removed; removing one frees the time again.
- R3 Availability can only be set at locations the practitioner works at (Practice R10) and is always clipped to
  the location's opening hours and closures when searching; it never extends them.

**Slot search**
- R4 Free slots for a service at a location over a date range: start times on the clinic's slot grid where the
  whole service duration fits inside the location's opening hours **and** a working block of a practitioner who
  is active, works at that location and has a role the service allows, and does not overlap a closure, a block or
  another non-cancelled appointment of that practitioner.
- R5 Search can be for one practitioner or "anyone": "anyone" returns each start time once, with the practitioners
  free at that time.
- R6 Slots are computed in the clinic's time zone (daylight saving included) and returned as UTC instants plus the
  local wall-clock time. Slots in the past or sooner than the minimum notice are not offered, nor beyond the
  booking window. **(assumption: minimum notice 2 hours, window 60 days, per clinic, Q4)**

**Appointments**
- R7 An appointment has a patient (a Weline contact, by `contact_id`), service, practitioner, location, start and
  end (end = start + service duration), status, source (`staff`, `web`, `whatsapp`; `cliniko` reserved for Phase
  4), a short booking reference unique in the clinic, an optional note (max 280 characters, administrative only),
  and who created it.
- R8 No double booking, enforced twice: the service refuses a time that overlaps another non-cancelled appointment
  of the same practitioner (the error names the clash), and a Postgres exclusion constraint on practitioner and
  time range refuses it even if two requests race.
- R9 Creating or moving an appointment re-runs the slot rules (R4, R6). Staff may override the hours and
  availability check (not the double-booking check) with an explicit "book outside hours" flag, which is
  recorded; patients never can. **(assumption: staff override allowed, Q5)**
- R10 Status machine: `BOOKED` → `CONFIRMED`, `ARRIVED`, `CANCELLED`, `NO_SHOW`; `CONFIRMED` → `ARRIVED`,
  `CANCELLED`, `NO_SHOW`; `ARRIVED` → `COMPLETED`. `CANCELLED`, `NO_SHOW` and `COMPLETED` never change. Any other
  change is refused naming the allowed ones. New appointments start as `BOOKED` from every source.
- R11 Every status change, and every move (time, practitioner or location), is written to
  `wl.booking_status_history` with old and new values, who did it (user or source) and an optional reason.
- R12 Only `BOOKED` and `CONFIRMED` appointments can be moved. Cancelling frees the time immediately.
- R13 Staff find a patient by name or mobile among the organization's contacts, or add a new one with name and
  mobile. A patient is never booked into two overlapping appointments (refused for patients and the chatbot;
  staff see a warning they can override). **(assumption, Q6)**

**Public booking page**
- R14 `/book/<slug>` works without login: it shows the clinic name, its active locations, services (with price
  when set) and practitioners (display name and role only), then free times (R4–R6) for the chosen service,
  location and optionally practitioner.
- R15 The patient enters name and Australian mobile, ticks the collection notice, and passes a captcha; the booking
  is created with source `web` and the page shows the reference, time, place and practitioner. The patient is
  matched to an existing contact by mobile or added as a new contact. **(assumptions: Q1 contact creation, Q3
  captcha provider, Q7 Australian mobiles only)**
- R16 The page and its API are rate limited per IP, and bookings are also limited per mobile per day. The API
  returns no internal ids beyond what the next request needs and nothing about other patients.
- R17 The page shows a collection notice (APP 5): which clinic collects the details, why, that they're stored in
  Weline, and a link to the clinic's website for its privacy policy.

**WhatsApp (chatbot `api_call`)**
- R18 `GET /api/x/booking/slots` (and a services list) can be called from a Weline chatbot `api_call` node with
  an API key, and have a chatbot view returning up to 10 options as flat fields a `buttons` node can use as list
  titles (`slot_1_label`, …, `slot_count`, plus an `offer_id` that remembers what was offered).
- R19 `POST /api/x/booking/appointments` can be called from an `api_call` node with the chosen slot id and the
  built-in `{{phone_number}}`; the patient is the contact with that number in the organization, source
  `whatsapp`. The response includes a ready-to-send confirmation line. A step-by-step guide for building the
  "Book an appointment" flow is written in this step (`chatbot-flow.md`), after prompt B.

**Access**
- R20 Permissions: `wl_booking_appointments` (read, create, update) and `wl_booking_availability` (read, update:
  practitioner hours and blocks). Appointments are never deleted (cancel instead). On install `admin` and
  `manager` get all; `agent` (reception) gets appointments read, create, update and availability read.
- R21 Every screen shows loading, empty, error and forbidden states, and read-only users see no edit controls.

**Replacing the prototype**
- R22 The legacy booking prototype (`internal/booking`, `frontend/src/views/booking`,
  `frontend/src/services/weline`, `frontend/src/router/booking.ts`, its two `Weline hook` lines and its allowlist
  and seam-exception entries) is removed. Its `public.booking_appointments` table is left in place (migrations are
  additive) and documented for manual removal. **(Q8)**

**For other modules**
- R23 A Go service other modules call (never HTTP between modules): search slots, create, move, change status,
  get and list appointments (by day, practitioner, contact), with the same rules and history as the API.

## Non-functional

- Every query is scoped to the active organization; another organization's appointments, availability, contacts
  and practice data can never be read, booked against or changed (tested with a second organization). Public
  routes find the organization only through the booking slug.
- Times are stored as UTC instants; slot maths uses the clinic's IANA time zone, tested across the April and
  October daylight-saving changes (Sydney) and in a zone without them (Brisbane, Perth).
- Slot search for one service, one location, 14 days and 10 practitioners answers in under 300 ms on staging.
- Patient data is administrative only (who, when, where, what service). No clinical notes; the note field says so.
  Phone numbers, names and notes are never logged.
- Screens use Weline's existing components and are usable with a keyboard; strings under `wl.booking.*`. The
  public page is usable on a phone and with a screen reader.

## Out of scope (this slice)

- Day and week calendar views, drag to move (Calendar 1.3). This slice has a list of appointments by day.
- Reminders and confirmation templates sent outside the 24-hour window (Phase 2); web bookings get no WhatsApp
  message in this slice. **(Q9)**
- Patients cancelling or rescheduling themselves (by link or WhatsApp).
- Recurring appointments, group classes, rooms and equipment, several practitioners per appointment, buffer time
  between appointments, different durations for new and returning patients.
- Waitlists, deposits and payments, Medicare and health-fund details, intake forms.
- Cliniko sync (Phase 4) and two-way calendar sync.
- Per-location booking links, clinic branding on the public page.
- Retention and deletion jobs for old appointments (rule recorded in `design.md`, job built before real patient
  data, SDLC privacy checklist). **(Q10)**

## Open questions

Defaults above are used until you answer. Please confirm or correct:

| # | Question | Default used |
| --- | --- | --- |
| Q1 | When someone books on the web page with a mobile Weline hasn't seen, should Weline add them as a contact (so they appear in the inbox and can get reminders later)? | Yes: add a contact on the clinic's default WhatsApp account; match existing contacts by mobile |
| Q2 | Do practitioners work set weekly hours that differ from the clinic's opening hours (part-time, different days at different sites)? | Yes: each practitioner has their own weekly blocks per location, always inside opening hours |
| Q3 | Captcha on the public page: Cloudflare Turnstile (free, no puzzles for most people, needs a free Cloudflare account) or another provider? | Turnstile. You'd create a site key and secret and add them in Render (step B tells you when) |
| Q4 | How far ahead can patients book, and how much notice does the clinic need? | Up to 60 days ahead, at least 2 hours' notice, set per clinic by the manager |
| Q5 | May reception book outside a practitioner's hours (squeeze-ins, after-hours)? | Yes, with an explicit "outside hours" tick that is recorded; double booking is never allowed |
| Q6 | Should a patient be stopped from holding two overlapping appointments (e.g. two practitioners at once)? | Refused for web and WhatsApp; staff get a warning they can override (family bookings under one contact) |
| Q7 | Only Australian mobiles on the public page? | Yes: `04xx xxx xxx` or `+61 4xx xxx xxx` |
| Q8 | The old booking prototype (`/booking` page, `booking_appointments` table) holds nothing real? | Remove its code; leave the empty table for a manual drop |
| Q9 | Should a web booking send a WhatsApp confirmation? (Needs an approved template, since the patient hasn't messaged the clinic.) | Not in this slice; added with the confirmation template in Phase 2 |
| Q10 | How long must appointment records be kept? | Keep 7 years after the appointment (common for adult health records in Australia), then delete; job built later |
