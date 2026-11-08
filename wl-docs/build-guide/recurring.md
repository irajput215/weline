# Recurring prompts

[← Back to the guide](README.md)

## Upstream sync (every 1 to 2 weeks)

You run:

```bash
git checkout main && git fetch upstream && git merge --ff-only upstream/main && git push
git checkout dev && git pull && git checkout -b sync/upstream-$(date +%F)
git merge main
```

If there are conflicts, paste:

```text
I'm merging upstream whatomate (main) into our dev branch on sync/upstream-<date>. Resolve the conflicts.
Upstream wins everywhere except our `// Weline hook` lines and anything under internal/modules,
frontend/src/modules, wl-docs/ and CLAUDE.md. Re-apply each hook line in the right place if upstream moved code.
Then run make test, the frontend tests and the server with -migrate on an empty database, and summarise what
upstream changed that could affect our modules (webhook payloads, message sending, auth, router, layout).
```

Then open a PR from the sync branch into `dev`.

## Release

```text
Prepare release v<YYYY.MM.DD> from dev to production:
1. List merged PRs since the last tag and write release notes in wl-docs/releases/<version>.md
2. List new migrations in this release and confirm each is additive
3. Check the AGPL "Source code" link points at this tag
4. Open a PR dev -> production titled "release v<YYYY.MM.DD>"
```

After merging: tag, back up the database, deploy, run `./whatomate server -migrate` once, then restart without `-migrate`, and watch logs for 48 hours.

## Bug fix

```text
Bug: <what happens, steps, expected>. Branch fix/<short-name> from dev.
Write a failing test that reproduces it first, then fix it, then show me the test passing.
Don't change anything unrelated.
```
