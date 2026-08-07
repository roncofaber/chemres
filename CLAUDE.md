# ChemRes — Project Instructions

## What this is

Go web app for resolving chemical identifiers (SMILES, CAS, names, InChIKeys, InChI) via the PubChem REST API. Deployed at chemres.app (Fly.io).

- **Stack**: Go stdlib `net/http`, `html/template`, htmx (single lookup), vanilla JS + SSE (batch)
- **Main branch**: `main`

## Release process

Version tags (`v*`) trigger the GitHub Actions deploy. Tag only after the CHANGELOG entry is written and pushed.

```bash
git tag vX.Y.Z && git push origin main vX.Y.Z
```

## Changelog discipline

Add a `CHANGELOG.md` entry under `## [Unreleased]` **as part of the same change** that makes something user-visible — not backfilled later. Promote `[Unreleased]` to a version section when tagging a release.

**Style**: one short line per entry, under ~12 words. State *what* changed for the user, never *why* or *how* — no root cause, no file/function/variable names, no hex colors, no parenthetical implementation asides. That detail belongs in the commit message. Group under `### Added`/`### Changed`/`### Fixed` (only sections that apply). A version can open with 1-2 sentences of prose summarizing the release before the bullets, if it helps orient the reader. Purely internal changes (refactors, CI tweaks) with no user-visible effect don't get an entry.

Bad: "Fix ECHA aggregated entry selection bug — compounds with multiple ECHA C&L entries were picking the wrong one, causing water to show H315/H319/H335 hazard statements that only applied to a 1% minority report."
Good: "Fixed wrong hazard classification showing for some compounds."

## Deployment

- Docker image: `debian:bookworm-slim` runtime (not `alpine`) — required for `openbabel`, which Alpine 3.20 dropped from its repositories
- `obabel` is a hard dependency for CDXML batch uploads (ChemDraw reaction files → SMILES)
