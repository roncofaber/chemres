# Changelog

## [v1.5.1] — 2026-07-10

### Fixed
- Add `openbabel` to Docker image — CDXML parsing was silently failing in production

---

## [v1.5.0] — 2026-07-10

### Added
- **CDXML support**: upload ChemDraw CDXML files in batch; structures extracted via OpenBabel and reaction roles (Reactant, Product, Reagent, Condition) assigned from the `<scheme>/<step>` XML
- **Role column** in batch results table and CSV export (only shown when roles are present)

### Changed
- **CSV restructure**: computed properties (`ExactMass`, `XLogP`, `TPSA`, `HBD`, `HBA`, `RotBonds`, `Stereocenters`, `Charge`, `Volume3D`) collapsed into a single `ComputedProperties` column
- **GHS export** collapsed into a single `GHS` column (`Signal=...; Pictograms=...; HCodes=...; PCodes=...`)
- Column order: `CommonName` before `IUPAC`, `ResolvedAt`/`Error` moved to end

---

## [v1.4.1] — 2026-07-01

### Fixed
- GHS SVG white polygon inserted at root SVG scope (was inside transform group, invisible in dark theme)
- Water and other non-hazardous compounds no longer show a Hazards section (ECHA entry selection bug fixed; H-statement filter now rejects non-H-code strings)
- Batch fetch options moved below Clear button (overlap fixed)
- Explicit `font-family` on all hazards section elements

---

## [v1.4.0] — 2026-07-01

### Added
- **GHS hazard data**: pictograms (self-hosted SVGs) displayed below structure image in a 3×3 grid; Hazards collapsible section with aligned H-statement layout, Precautionary codes, and ECHA C&L Inventory source link
- **Batch GHS opt-in**: checkbox in batch form fetches GHS data per compound (parallel, post-batch); exported to CSV when present
- **Batch Synonyms opt-in**: checkbox to control whether CAS and synonyms are fetched

### Fixed
- Multi-fragment SMILES now render fully (ionic compounds, salts) — removed largest-fragment filter
- `looksLikeSMILES` now checks for spaces first; SMILES resolver falls back to name resolver on parse failure
- `Semax (acetate)` and similar space-containing names no longer show "Could not reach PubChem"
- SmilesDrawer attribution removed from footer (MIT licence, credited in README)

---

## [v1.3.1] — 2026-06-30

### Fixed
- ECHA aggregated GHS entry selection bug (compounds with multiple ECHA entries picked wrong one)
- GHS pictogram icons repositioned below PNG/Download buttons, 3×3 grid at 38px

---

## [v1.3.0] — 2026-06-30

### Added
- **GHS classification**: fetched concurrently from ECHA C&L Inventory; pictograms below structure, Hazards collapsible section
- GHS pictogram SVGs (GHS01–GHS09) self-hosted under `static/ghs/`

---

## [v1.2.2] — 2026-06-30

### Fixed
- Names with spaces (e.g. `Semax (acetate)`) routed correctly away from SMILES resolver
- SMILES resolver falls back to name resolver on `errBadInput`

---

## [v1.2.1] — 2026-06-30

### Fixed
- Multi-fragment SMILES rendered fully (was showing only largest fragment)
- SmilesDrawer link removed from footer

---

## [v1.2.0] — 2026-06-23

### Added
- **"Did you mean?" suggestions** on failed name lookups (PubChem autocomplete)
- Versioning via `git describe --tags`; version shown in footer and used for cache-busting

### Fixed
- CI deploy triggers on `v*` tags only; uses `github.ref_name` for clean version string

---

## [v1.1.0] — 2026-06-22

### Added
- Structure download menu: SVG, PNG, SDF 3D, XYZ (server-side SDF→XYZ conversion)
- Copy PNG button alongside Download
- Excel (`.xlsx`) file upload for batch
- Compact batch UI: filter and Export CSV on same row, secondary button style
- Type scale CSS variables; full JS extracted to `app.js` with event delegation and CSP

---

## [v1.0.0] — 2026-06-17

### Added
- Initial release: single lookup and batch resolution of SMILES, names, CAS, InChIKey, InChI via PubChem REST API
- Per-IP rate limiting (token bucket) with PubChem `X-Forwarded-For` routing
- Structure rendering via SmilesDrawer; structure modal with Copy/Download
- Computed physicochemical properties (XLogP, TPSA, HB donors/acceptors, rotatable bonds, stereocenters, charge, volume)
- Synonyms collapsible; CAS resolution
- Dark/light theme toggle with cookie persistence
- GitHub Actions CI/CD deployment
