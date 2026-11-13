# Onboarding: API

## Public (no login, outside `/api`)

| Method | Path | Body | Returns |
| --- | --- | --- | --- |
| GET | `/x/onboarding/signup` | | HTML page; 404 page when sign-up is closed |
| POST | `/x/onboarding/public/signup` | `{business_name, full_name, email, password, time_zone, accept_terms, captcha_token}` | `{organization_id, email}` |
| GET | `/x/source` | | 302 to the source of the running build |

Sign-up errors: 404 closed (`WELINE_SIGNUP` not `open`), 503 no robot check configured, 400 invalid form or robot
check failed, 409 email already has an account, 429 more than 5 an hour from one address.

## Staff (`/api/x/onboarding`, Weline envelope)

| Method | Path | Permission | Body | Returns |
| --- | --- | --- | --- | --- |
| GET | `/checklist` | `wl_onboarding_setup` read | | `Checklist` |
| PUT | `/vertical` | `wl_onboarding_setup` update | `{vertical}` | `Checklist` (audited `vertical_set`) |
| PUT | `/dismissed` | `wl_onboarding_setup` update | `{dismissed: bool}` | `Checklist` |

```jsonc
// Checklist
{
  "vertical": "allied_health",   // '', allied_health, dental, medical, beauty, fitness, other
  "dismissed": false,
  "steps": [ { "key": "business", "done": true, "link": "/x/practice/profile" } /* whatsapp, system, vertical, team, first_message */ ],
  "done": 1,
  "total": 6
}
```

## Environment

`WELINE_SIGNUP=open`, `WELINE_TURNSTILE_SITE_KEY` / `WELINE_TURNSTILE_SECRET` (shared with booking),
`WELINE_SOURCE_URL` (optional), `WELINE_TERMS_URL`, `WELINE_PRIVACY_URL`.
