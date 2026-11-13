# Ads: release notes

## 2.3: Entry points and where patients come from (first release)

Shipped in Weline Phase 2.3. Spec: [`requirements.md`](requirements.md). Design: [`design.md`](design.md).
API: [`api.md`](api.md). Why codes and not Meta's ad data: [`spike.md`](spike.md).

### What's new

An **Ads** section in the sidebar with **Entry points**.

- **Entry points:** give each ad, flyer, QR code, email or website button its own WhatsApp link.
  - **Name and code:** the code is suggested from the name, for example "Flu shot ad 2024" → `FLU24`. It uses 3 to 12
    letters and digits, with at least one of each.
  - **Where it's used:** Facebook or Instagram ad, Google ad, website, QR code or flyer, email, other.
  - **An optional campaign** that groups entry points.
  - **Your WhatsApp number:** remembered in your browser for next time.
  - **The message people send,** which carries the code, for example "Hi! I'd like to know more [FLU24]".

  Weline gives you the WhatsApp link and the message, each with a copy button. For a Facebook or Instagram
  click-to-WhatsApp ad, paste the message into the ad's pre-filled message.
- **Arrivals:** when someone sends a message with the code (in any letter case, alone or in brackets), Weline records
  where they came from and whether they're new. A contact counts as new if their first message was at most 10 minutes
  earlier. Their contact gets a `from-flu24` tag and a `campaign-…` tag.
  - Both tags join your tag list, so you can build an Audiences segment of everyone who came from an ad or campaign.
  - Sending the code again the same day doesn't count twice.
  - Weline sends nothing back: your chatbot and inbox carry on as usual.
- **Counts** for the last 7, 30 or 90 days or your own dates, per entry point and per campaign:
  - chats started;
  - new contacts;
  - **opted in**: agreed to marketing in Consent within 30 days of arriving;
  - **booked**: booked in Weline's Booking within 30 days.

  Opt-ins and bookings go to the last entry point the person came through.
- **Each entry point's page:** its link, message, tags and counts, and the people who arrived, with masked numbers.
- **Edit** anything but the code. **Archive** an entry point to stop counting its code; its history stays. **Delete**
  one nobody has arrived through yet.

Arriving through an ad is not consent to marketing: opt-ins still come only from Consent.

### Who can do what

| Permission | Admin | Manager | Agent (reception) |
| --- | --- | --- | --- |
| `wl_ads_entrypoints` (view, create, edit, archive and delete entry points) | read, create, update, delete | read, create, update, delete | – |

Reception sees where a patient came from through the tag on the contact.

### Upgrade notes

- Migration `ads/0001_create_ads` adds `wl.ads_entrypoints` and `wl.ads_arrivals` and grants the permission above to
  the default roles. It adds no columns to Weline's tables. It adds tags to contacts and to the tag list.
- Organizations created after the upgrade need `wl_ads_entrypoints` granted in the role editor (same as other
  modules).
- No WhatsApp templates needed.

### Known limits

- People who delete the code from the message aren't counted, so counts are a minimum.
- No QR code images yet: paste the link into any QR generator.
- No Meta ad data (ad id, ad name, click id), ad spend or cost per booking, and no Cliniko bookings (Phase 3).
- The chat shows the message as typed, code included.
