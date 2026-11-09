# Practice: requirements

**For:** the clinic's practice owner or manager (sets it up), reception (reads it every day), and every later
module (Booking, Calendar, Reminders, AI, Connectors), which reads who the clinic is and when it is open.

Status: **draft for review.** Lines marked **(assumption)** are defaults chosen without pilot-clinic input; each
one is listed under [Open questions](#open-questions) for you to confirm or correct before step B (build).

## User stories

- As a practice owner, I enter my clinic's name, time zone, contact details and booking link name once, so that
  patients and every Weline feature use the right details.
- As a practice owner, I add my locations, so that appointments and hours belong to the right site.
- As a practice owner, I list my practitioners, link the ones who use Weline to their login, and say where each
  one works, so that bookings can be made with the right person at the right place.
- As a practice owner, I list the services we offer, how long each takes, what it costs and which kinds of
  practitioner can provide it, so that booking offers only valid combinations.
- As a practice owner, I set each location's opening hours and add closures (public holidays, leave days), so
  that nobody is booked while the clinic is shut.
- As a receptionist, I can see all of this but not change it, so that I can answer patients without risking the setup.

## Functional requirements

**Clinic profile**
- R1 Each organization has one clinic profile: display name, IANA time zone, booking slug, slot length, and
  optionally ABN, phone, email and website. It is created on first save; until then the module reports
  "not set up".
- R2 The time zone is one of the Australian zones (`Australia/Sydney`, `Melbourne`, `Brisbane`, `Adelaide`,
  `Darwin`, `Perth`, `Hobart`), default `Australia/Sydney`. **(assumption: one time zone per clinic, Q5)**
- R3 The booking slug is 3–40 characters of lowercase letters, digits and hyphens, unique across all of Weline,
  and not one of a reserved list (`admin`, `api`, `login`, `x`, …). It will form the public booking link
  `/book/<slug>` (built in 1.2). **(assumption: one slug per clinic, Q6)**
- R4 The slot length is one of 5, 10, 15, 20, 30 or 60 minutes, default 15. It is the grid for opening hours
  and service durations.
- R5 The ABN, when given, is 11 digits and passes the ABN checksum.

**Locations**
- R6 A clinic has one or more locations: name (unique in the clinic), address and phone. Booking needs at
  least one active location.
- R7 A location can be deactivated (hidden from new bookings) and reactivated; locations are never deleted.

**Practitioners**
- R8 A practitioner has a display name, a role (e.g. "GP", "Nurse", "Physiotherapist"), and optionally an AHPRA
  registration number (3 letters + 10 digits) and a link to a Weline user. **(assumption: practitioners need
  not have a Weline login, Q4)**
- R9 A Weline user can be linked to at most one practitioner in a clinic, and only a user who belongs to that
  clinic's organization.
- R10 A practitioner works at one or more of the clinic's locations.
- R11 Practitioners can be deactivated and reactivated; they are never deleted.

**Services**
- R12 A service has a name (unique in the clinic), a duration that is a whole number of slots (between 1 slot
  and 8 hours), an optional price in AUD (cents, including GST), and the practitioner roles allowed to provide
  it (at least one). **(assumption: fixed duration and price per service, Q2)**
- R13 Services can be deactivated and reactivated; they are never deleted.

**Opening hours and closures**
- R14 Each location has opening hours per weekday: closed, or one opening and one closing time on the slot grid
  (closing after opening). A location with no hours set is closed every day. **(assumption: one block per day,
  no lunch break, the same every week, Q3)**
- R15 Closures block whole days for one location or for all locations, between two dates inclusive, with a
  reason (e.g. "Good Friday"). Public holidays are entered as closures; they are not imported automatically.
  Past closures stay listed for reference.

**Access**
- R16 Permissions: `wl_practice_settings` (read, update: profile, hours, closures), `wl_practice_locations`,
  `wl_practice_practitioners` and `wl_practice_services` (read, create, update each). On install the system
  `admin` and `manager` roles get all of them and `agent` (reception) gets read only. Admins can change this in
  Weline's role editor.
- R17 Every screen shows loading, empty, error and forbidden states, and users with read-only access see the data
  without edit controls.

**For other modules**
- R18 The module offers a Go service other modules call (never HTTP between modules): clinic profile, active
  locations, active practitioners with their locations, active services, and "is location L open at instant T"
  using the clinic's time zone and closures.

## Non-functional

- Every query is scoped to the active organization; another organization's locations, practitioners, services,
  hours and users can never be read, linked or changed (tested with a second organization).
- Opening hours are stored as local wall-clock times plus the clinic's IANA time zone, so daylight-saving changes
  need no data edits. Instants (created/updated) are UTC.
- Screens use Weline's existing components and are usable with a keyboard; strings under `wl.practice.*`.
- No patient data is stored by this module.

## Out of scope (this slice)

- Per-practitioner availability, rosters and leave (Booking 1.2: `wl.booking_availability`, `wl.booking_blocks`).
- Lunch breaks or several opening blocks per day; hours that differ week to week. **(Q3)**
- Several time zones within one clinic. **(Q5)**
- Automatic public-holiday import (per state).
- Practitioner photos, bios and public profiles; clinic logo and branding on the booking page.
- Online payments, deposits, Medicare item numbers and health-fund rebates on services.
- Syncing practitioners and services from Cliniko (Phase 4).
- Deleting anything (deactivate instead), and an audit trail of changes (threat model open risk 7).

## Open questions

Defaults above are used until you answer. Please confirm or correct:

| # | Question | Default used |
| --- | --- | --- |
| Q1 | What kind of clinic is the pilot (GP, physio, dental, allied health), and roughly how many practitioners and locations? | Small multi-practitioner clinic, 1–3 locations; nothing in this slice depends on the type |
| Q2 | Do services have a fixed duration and price, and are some offered only by certain practitioners? | Fixed duration and price per service; restricted by practitioner **role**, not by named practitioner |
| Q3 | Are opening hours the same every week? Lunch closures? How are public holidays handled today? | Same every week, one block per day, public holidays entered as closures |
| Q4 | Will every practitioner have a Weline login, or are some bookable without one? | Login optional; linking a login is optional |
| Q5 | Are all locations in one state, or across states? | One time zone per clinic |
| Q6 | One public booking link per clinic, or per location? | One per clinic; the patient picks the location on the booking page |
