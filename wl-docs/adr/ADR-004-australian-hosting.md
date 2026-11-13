# ADR-004: Host staging and production in Australia
Date: 2026-11-13 · Status: **proposed: you choose the host**; this is the migration plan.

## Context

Staging runs on Render's free tier in Oregon (US), and Render's free Postgres expires on 8 December 2026. Patient
information (names, mobiles, messages, appointments) must be hosted in Australia before a real clinic's data flows
(SDLC "Compliance", build guide step 3.3, threat model open risk 6).

Weline needs: one Go web service (the whatomate binary with modules, `-migrate` on deploy), Postgres 16, Redis 7,
object storage for media (S3-compatible), HTTPS on a public address (for Meta's webhook and `WELINE_PUBLIC_URL`), and
environment variables and secrets.

## Options (verify current regions and prices before choosing)

| Option | Web service | Postgres | Redis | Notes |
| --- | --- | --- | --- | --- |
| A. **AWS Sydney** (`ap-southeast-2`) | App Runner or ECS Fargate | RDS for PostgreSQL | ElastiCache | Everything in one Australian region and account; most setup work; strongest compliance story |
| B. **Fly.io Sydney** (`syd`) + managed Postgres in Sydney | Fly Machines in `syd` | Fly's managed Postgres, or Neon on AWS Sydney | Upstash Redis (Sydney region) | Closest to Render's simplicity; check each provider's data-location terms |
| C. **DigitalOcean Sydney** (`SYD1`) | App Platform | Managed PostgreSQL | Managed Redis/Valkey | One provider, simple; check App Platform region availability |
| D. Render Singapore | – | – | – | Not Australia; doesn't meet the requirement |

## Recommendation

**B or C** for staging now (low effort, Australian region), and **A** for production once there are paying clinics, if
a clinic's privacy review asks for it. Whatever you choose, keep media storage in the same region.

## Migration plan (any option)

1. Create the database (Postgres 16) and Redis in the Sydney region; create the S3-compatible bucket in the same region.
2. Create the web service from the repository's Dockerfile, with the same environment variables as `render.yaml`
   (including `WHATOMATE_APP__ENCRYPTION_KEY`, the JWT secret, `WELINE_PUBLIC_URL` set to the new address,
   `TZ=Australia/Sydney` (step 2.4), `WELINE_CLINIKO_CONTACT_EMAIL`), and the start command with `-migrate`.
3. Staging has no real patient data: start with an empty database (migrations create everything) rather than copying.
   For production later, dump and restore with `pg_dump`/`pg_restore` during a short maintenance window.
4. Point the Meta app's webhook URL and any custom domain at the new address; check `/health`.
5. Re-run the module journeys against the new staging and the real-phone checks in each module's DoD.
6. Shut down the Render services; delete the Render database after confirming nothing reads it.

**You:** choose the host and create the account; then I can turn this into a deploy blueprint for that host.
