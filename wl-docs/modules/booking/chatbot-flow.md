# Booking: the "Book an appointment" WhatsApp flow

How to build the WhatsApp booking flow in Weline's chatbot flow builder (Chatbot → Flows), using Booking's
chatbot endpoints. The patient picks a service, Weline offers the next free times as a list, the patient taps one,
and the booking is made and confirmed in the chat. Requirements R18 and R19; API in [`api.md`](api.md).

Every request below was replayed against a running Weline with a real API key before this guide was written
(Go test `TestAPI_ChatbotFlow` covers the same calls).

```mermaid
flowchart TD
    T([Patient sends "book"]) --> A1[API: Get services]
    A1 -- error --> E1[Text: booking unavailable] --> X1([End])
    A1 -- success --> C1{Condition: any services?}
    C1 -- false --> E1
    C1 -- true --> B1[Buttons: choose a service]
    B1 --> L{More than one location?}
    L -- "yes: add" --> B2[Buttons: choose a location]
    L -- no --> A2
    B2 --> A2[API: Find times]
    A2 -- error --> E1
    A2 -- success --> C2{Condition: any times?}
    C2 -- false --> E2[Text: no times] --> X1
    C2 -- true --> B3[Buttons: choose a time]
    B3 --> A3[API: Book]
    A3 -- success, sends confirmation --> X2([End])
    A3 -- error --> E3[Text: just taken] --> A2
```

## Before you start

1. **Booking is set up** for the clinic: Practice profile, at least one location with opening hours, practitioners
   with hours (Bookings → Availability), and services. If staff can see free times in **New appointment**, the
   flow will too.
2. **Know your Weline address.** Flows call Weline over the internet through its public HTTPS address, never
   `localhost` (Weline's network guard blocks local addresses). Below it's written as `https://WELINE`; use your
   real address, e.g. `https://weline-staging.onrender.com` (the same value as `WELINE_PUBLIC_URL`).
3. **Create a "WhatsApp bot" login with only booking rights.** An API key has all the rights of the user who
   created it, and anyone who can edit flows can read the key, so it must belong to a user who can do nothing else.
   1. **Settings → Roles → New role** "WhatsApp bot" with `wl_booking_appointments`: **read** and **create**, plus,
      for now, `api_keys`: **write** (a user can only create keys for themselves).
   2. **Settings → Users → New user**, e.g. `whatsapp-bot@yourclinic.com.au`, role "WhatsApp bot", in the clinic's
      organization.
   3. Log in **as that user**, open **Settings → API Keys**, create a key named "WhatsApp booking flow", and copy it.
      It's shown once. Log out.
   4. Back as an admin, edit the "WhatsApp bot" role and **remove** `api_keys`: **write**. Permissions are checked on
      every request, so the key keeps working with booking rights only.

## Build the flow

**Chatbot → Flows → New flow.** In **Flow Settings**, name it "Book an appointment" and set **Trigger Keywords** to
`book, booking, appointment`. Then add these nodes from the palette, in order. Node names (the **Label** field) are
suggestions; variable names must match exactly, because later nodes use them.

Every **API** node gets the same header: **Headers** → Key `X-API-Key`, Value `<the bot's key>`. An API node has
two outputs: **success** (2xx) and **error** (anything else). Response mapping only runs on success.

### 1. API: "Get services"

| Field | Value |
| --- | --- |
| URL | `https://WELINE/api/x/booking/catalog?view=chatbot` |
| Method | `GET` |
| Headers | `X-API-Key`: `<key>` |

**Response mapping** (variable → path):

| Variable | Path |
| --- | --- |
| `catalog_offer` | `data.offer_id` |
| `service_count` | `data.service_count` |
| `service_1_label` … `service_10_label` | `data.service_1_label` … `data.service_10_label` |
| `location_count` | `data.location_count` |
| `location_1_label` … `location_10_label` | `data.location_1_label` … `data.location_10_label` (only if the clinic has more than one location) |

Add as many `service_N_label` rows as the clinic has services (at most 10; the first 10 active services by name are
offered). Labels are cut to 24 characters, WhatsApp's limit for a list row.

**error** → a **Text** node "Sorry, online booking isn't available right now. Please call the clinic." → **End**.

### 2. Condition: "Any services?"

Expression: `service_count > 0`. **true** → node 3. **false** → the same "isn't available" text.

### 3. Buttons: "Choose a service"

- **Body:** `What would you like to book?`
- **Button Options:** one **Reply** button per service, with these exact titles and ids:

  | Button Title | button_id |
  | --- | --- |
  | `{{service_1_label}}` | `service_1` |
  | `{{service_2_label}}` | `service_2` |
  | … | … |

  Up to 10. Buttons whose title comes out empty are skipped, so extra rows are harmless. With more than 3 buttons
  WhatsApp shows them as a list.
- **Store response as:** `service_choice`. This saves the tapped title, which the next call uses.
- On the canvas, drag **every** button's handle to the next node. A button with no wire ends the flow.

### 3b. Buttons: "Choose a location" (only clinics with more than one location)

Same as node 3 with body `Which clinic?`, titles `{{location_1_label}}` … ids `location_1` …, and **Store response
as** `location_choice`. A single-location clinic skips this node: the next call then uses its only location.

### 4. API: "Find times"

| Field | Value |
| --- | --- |
| URL | `https://WELINE/api/x/booking/slots/chatbot` |
| Method | `POST` |
| Headers | `X-API-Key`: `<key>` |
| Body | see below |

```json
{"offer_id": "{{catalog_offer}}", "service_label": "{{service_choice}}", "location_label": "{{location_choice}}"}
```

Use the **POST** call, not `GET /slots?...`: Weline puts variables into URLs as they are, so a service name with a
space, `&` or `#` would break a URL. In a JSON body they're safe. Avoid double quotes `"` in service and location
names, because they would break the JSON.

**Response mapping:**

| Variable | Path |
| --- | --- |
| `times_offer` | `data.offer_id` |
| `slot_count` | `data.slot_count` |
| `slot_1_label` … `slot_10_label` | `data.slot_1_label` … `data.slot_10_label` (all 10) |
| `times_message` | `data.message` |

Times follow the clinic's online booking rules (minimum notice and how far ahead, under Bookings → Availability).
Each label reads like `Wed 11 Nov 09:00 Dr Lee`: the time and the first practitioner free then.

**error** → the "isn't available" text. The most common causes are under *Troubleshooting*.

### 5. Condition: "Any times?"

Expression: `slot_count > 0`. **true** → node 6. **false** → a **Text** node
`{{times_message}} Reply "book" to try another service, or call the clinic.` → **End**.

### 6. Buttons: "Choose a time"

- **Body:** `{{times_message}}`
- **Button Options:** 10 **Reply** buttons, titles `{{slot_1_label}}` … `{{slot_10_label}}`, ids `slot_1` … `slot_10`.
- **Store response as:** `time_choice`
- Wire **all 10** buttons to node 7.

### 7. API: "Book"

| Field | Value |
| --- | --- |
| URL | `https://WELINE/api/x/booking/appointments` |
| Method | `POST` |
| Headers | `X-API-Key`: `<key>` |
| Body | see below |

```json
{"offer_id": "{{times_offer}}", "choice": "{{time_choice}}", "phone": "{{phone_number}}"}
```

`{{phone_number}}` is built in: the WhatsApp number of the person in the chat. The booking is always for that
contact, in the bot's organization, with source **WhatsApp**.

**Response mapping:**

| Variable | Path |
| --- | --- |
| `confirmation` | `data.confirmation` |
| `booking_ref` | `data.appointment.reference` |

**Message template:** `{{confirmation}}`. It's sent when the booking succeeds, e.g.
`Booked: Standard consult with Dr Lee, Wed 11 Nov 09:00 at Bondi. Ref KYKCRF`.

**success** → **End** (optionally with "See you then! Reply 'book' to make another booking.").

**error** → a **Text** node `Sorry, that time was just taken or the list expired. Here are the latest times.` →
back to node 4 ("Find times"). Offered times are held for 30 minutes; someone else may book a time in between, and
the booking is always re-checked.

Save and **activate** the flow.

## Try it

1. From a test phone, send **book** to the clinic's WhatsApp number (staging: the test number).
2. Pick a service, then a time.
3. Check the confirmation arrives, then find the appointment under **Bookings → Appointments** on that day, with
   source **WhatsApp** and the same reference.
4. Book again and tap a time someone else has meanwhile taken (book it from Weline first): you should get "just
   taken" and a fresh list.

Bookings made this way follow every booking rule: hours, time off, the clinic's notice and window, no double
booking, and no two appointments at once for the same patient.

## Troubleshooting

When an API node takes its **error** output, the Render logs show `Configured API request returned non-2xx` with
the status and a short response preview (no patient data).

| Status and message | Cause | Fix |
| --- | --- | --- |
| 401 | Missing or wrong `X-API-Key` | Re-copy the key into every API node; check it wasn't revoked |
| 403 Insufficient permissions | The bot's role lacks `wl_booking_appointments` read/create | Edit the "WhatsApp bot" role |
| 409 practice not set up | No Practice profile | Clinic → Practice profile |
| 400 location is required (the clinic has N locations) | Several locations but no location step | Add node 3b |
| 400 "…" isn't one of the options | The service or time title didn't match the list | Check **Store response as** names; don't edit button titles |
| 409 those options have expired | More than 30 minutes between list and tap | The error path in node 7 already re-offers times |
| 409 sorry, that time was just taken | Someone booked it first | Same; expected |
| 409 the patient already has an appointment at … | The patient is already booked then | Staff can book overlaps in Weline if needed |
| 404 we couldn't find your details | No contact with that number in the bot's organization | The bot user must be in the same organization as the WhatsApp number |
| Request failed / blocked address | URL uses `localhost` or a private address | Use the public HTTPS address |

## Security notes

- The key sits in the flow's settings, readable by anyone who can edit flows (threat model open risk 4). That's why
  it belongs to a login that can only see and book appointments. To rotate it, create a new key as the bot user,
  paste it into the three API nodes, then delete the old key (give the role `api_keys` write
  again for that, then remove it).
- The flow can only book for the person messaging (`{{phone_number}}`); it can't read other patients' bookings.
- Weline logs only the path of these calls, never the body, so phone numbers stay out of the logs as long as
  `WHATOMATE_APP__DEBUG` stays `false`. Debug mode logs every SQL query with its values; never turn it on where
  there's real patient data.

## Reference: responses

`GET /api/x/booking/catalog?view=chatbot`

```json
{"status": "success", "data": {"offer_id": "c6d50993fd6a441926", "service_count": 2,
  "service_1_label": "Standard consult", "service_2_label": "Long consult", "service_3_label": "", "…": "",
  "location_count": 1, "location_1_label": "Bondi", "location_2_label": "", "…": ""}}
```

`POST /api/x/booking/slots/chatbot`

```json
{"status": "success", "data": {"offer_id": "9f2c0e41ab7d33c510", "slot_count": 10,
  "slot_1_label": "Wed 11 Nov 09:00 Dr Lee", "…": "", "slot_10_label": "Wed 11 Nov 11:15 Dr Lee",
  "message": "Here are the next available times:"}}
```

`POST /api/x/booking/appointments` (chatbot form)

```json
{"status": "success", "data": {"appointment": {"reference": "KYKCRF", "status": "BOOKED", "source": "whatsapp", "…": "…"},
  "confirmation": "Booked: Standard consult with Dr Lee, Wed 11 Nov 09:00 at Bondi. Ref KYKCRF"}}
```
