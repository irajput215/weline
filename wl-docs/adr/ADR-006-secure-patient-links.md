# ADR-006: Patient pages behind unguessable, expiring links served by the module
Date: 2026-11-21 · Status: proposed

## Context

Receipts ([`../modules/receipts/`](../modules/receipts/requirements.md)), and possibly medical documents later
([research](../research/whatsapp-patient-pages.md)), send a patient a WhatsApp message with a button that opens a page.
The page shows information about a health service the patient received, which is health information under the Privacy
Act. Patients have no Weline login. Choices that are hard to undo once links are in patients' chats:

- where the page is served (Vue app, which needs a sixth seam, or the module);
- what the link carries (an id, a signed token, a random token) and what Weline stores;
- whether the page reads the document live from the connected system or from a copy kept in Weline;
- whether opening it needs a second check beyond having the link.

## Decision

1. **Served by the module**, like the public booking page: a small Go `html/template` page outside `/api`
   (`/r/{token}` for receipts), no external scripts, no new seam.
2. **Random token, stored hashed.** 32 random bytes, base64url (43 characters), as the URL button's variable. Weline
   stores only its SHA-256, so a database copy can't open pages. Not a signed token: a random one can be revoked and
   looked up without a signing key to rotate.
3. **Expiring and revocable.** Each link has an expiry (receipts: 30 days by default) and staff can revoke it. Expired,
   revoked and unknown links all show the same "this link has expired, ask the clinic for a new one" page.
4. **Live, not copied.** The page fetches the document from the connected system when opened (cached briefly), so
   Weline keeps only which document a link points to, not its contents, and a paid invoice shows as paid.
5. **Locked-down responses:** `Cache-Control: no-store`, `Referrer-Policy: no-referrer`, `X-Robots-Tag: noindex`, a
   CSP allowing nothing but the page's own inline styles, `frame-ancestors 'none'`; rate limited per IP; every view
   recorded in the audit log without personal data.
6. **Second check by document kind.** Receipts open with the link alone (Q2 in the receipts requirements). Clinical
   documents, if ever built, must also ask for something only the patient knows (date of birth or a one-time code)
   before showing anything; that is a condition of building them, not an option.

## Consequences

- No upstream change; any module can reuse the pattern (a helper in `kit` once a second module needs it).
- Anyone holding the link sees the receipt until it expires or is revoked: a forwarded message or a shared phone
  exposes it. Accepted for receipts (the same as an emailed invoice), not for clinical documents (point 6).
- Tokens are in URL paths, so they appear in the host's request logs (Render keeps request paths). Mitigated by expiry
  and hashing in the database; worth testing whether Meta accepts a URL fragment (`#token`, never sent to the server)
  for the button variable. Open risk until then.
- Live reads depend on the connected system being up and count against its rate limit (Cliniko: 200 a minute per
  key). A down system shows "try again later" rather than stale data.
- To undo: stop sending new links and let old ones expire; the page and table can then be removed.
