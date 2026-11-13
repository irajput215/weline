# ADR-003: Scrub patient identifiers from the process's output
Date: 2026-11-13 · Status: **proposed, built and on by default** (turn off with `WELINE_SCRUB_LOGS=off`); confirm or
change it.

## Context

Upstream whatomate logs phone numbers on every send (`Text message sent … phone=…`) and sender numbers and profile names
on every incoming message (`Received message from=… profile_name=…`), at info level (threat model open risk 9). The
hosting provider (Render) stores that output. Before the first real clinic, patient identifiers must stay out of logs
(build guide step 3.3).

The log level is hard-coded in upstream's `cmd/whatomate/main.go` (Debug, or Info in production); there's no setting,
and Weline doesn't edit upstream code outside the five seams.

## Options

1. **Log level in production:** not possible without editing upstream (no configuration), and Warn would also hide
   useful Info lines.
2. **An upstream pull request** to stop logging identifiers or add a log level setting: the right long-term fix, but
   it waits on upstream review, and you've chosen not to sync with upstream until the deliverables are built.
3. **A scrubber in the deploy command** (`whatomate … | scrub`): needs a second binary in the image, which upstream's
   Dockerfile builds; changing it is an upstream file change.
4. **Scrub in the process (chosen):** when Weline's modules start (the existing `modules.Register` seam), Weline
   replaces the process's standard output and standard error (where upstream's logger, logf, writes) with pipes,
   masks identifiers in each line, and writes the result to the original outputs. No upstream file changes.

## Decision

Option 4, on by default, until option 2 lands upstream.

- **What's masked:** the value after `phone=`, `from=`, `to=`, `wa_id=`, `profile_name=`, `contact_name=`,
  `contact_phone=`, `recipient…=`, `mobile=`, `user_phone=`, `customer_phone=` (quoted or not, colour codes allowed),
  becomes `[redacted]`. Any other run of 9 to 15 digits that isn't part of an id, path, timestamp or word is masked to
  its last three digits.
- **What isn't:** ids (UUIDs), timestamps with separators, durations, file paths, URLs. A bare 13-digit number (for
  example a millisecond timestamp) is masked: it can't be told from a phone number, and privacy wins.
- **Where:** `internal/modules/kit/logscrub*.go`, started from `internal/modules/registry.go`. Linux only (production);
  a no-op elsewhere.

## Consequences

- Output written before modules start (startup lines) isn't scrubbed; those lines hold no patient data.
- Both standard output and standard error are scrubbed. A first version scrubbed only standard output; a run of the
  production build showed upstream's logger writes to standard error, so both are covered and tested.
- If the copier fails, it writes one notice to the original output; the app keeps running, but later output would
  block on a full pipe. Tested in a child process; watch for the notice in staging logs.
- Debug mode (`WHATOMATE_APP__DEBUG=true`) logs SQL with values; scrubbing masks phone-like values but not names in
  SQL. Keep debug off wherever there's real data (as the threat model already says).
- Revisit when upstream stops logging identifiers (send option 2 when syncing resumes).
