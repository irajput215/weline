// Package ads records where patients come from: each ad, website button,
// flyer or QR code is an entry point with a code in its WhatsApp link's
// pre-filled message. A message.incoming with an active code records an
// arrival and tags the contact; reports count conversations, new contacts,
// opt-ins and bookings per entry point and campaign. It sends nothing.
// Spec: wl-docs/modules/ads.
package ads

// Name is the module name used for migrations and logs.
const Name = "ads"
