# Changelog

## [Unreleased]

## [v1.6.1] — 2026-08-07

### Added
- Search engines and link previews now show a proper page description and preview image
- A short "what is this" tooltip on the homepage tagline

### Fixed
- Dragging to rotate a 3D structure no longer closes the viewer if you release the mouse outside it
- Structure image on the result card is now larger and square on narrow screens, and stays correctly sized when resizing the window
- 2D/3D toggle no longer overlaps the CID row on narrow screens

## [v1.6.0] — 2026-08-07

View compounds in 3D, plus a round of batch, accessibility, and reliability improvements.

### Added
- View compounds in 3D — toggle between 2D and 3D on the result card and in the structure viewer
- Batch: Ctrl/Cmd+Enter in the identifiers box resolves without clicking the button
- Batch: helpful message when nothing in a batch resolves
- Batch: capped at 1000 identifiers per run, with a clear message if exceeded
- Batch: duplicate identifiers now resolve once and share the result, instead of repeating the lookup
- CAS numbers are now validated by check digit — a mistyped CAS is caught immediately instead of returning "not found"

### Fixed
- Copying a 3D structure image to the clipboard now actually works
- Dragging to rotate a 3D structure no longer closes the viewer if you release outside it
- Switching light/dark theme while viewing a 3D structure no longer duplicates the image
- Toggling between 2D and 3D no longer resizes the structure card or viewer
- Server now shuts down cleanly on deploy instead of dropping in-flight requests
- Excel file handle leak on batch upload
- Container no longer runs as root
- Structure modal now returns keyboard focus to where you opened it from, and keeps Tab navigation inside the modal while open
- Screen readers now announce when search suggestions appear

## [v1.5.2] — 2026-07-10

Upload ChemDraw reaction files and resolve every compound in them at once, plus a leaner CSV export.

### Added
- Upload a CDXML (ChemDraw) file to batch-resolve every reactant, product, and reagent in a reaction
- Batch results and CSV export show each compound's role (Reactant, Product, Reagent, Condition)

### Changed
- CSV export: computed properties now share a single column instead of nine
- CSV export: GHS hazard data now shares a single column instead of four

### Fixed
- CDXML uploads failing in production

---

## [v1.4.1] — 2026-07-01

### Fixed
- Hazard pictograms not showing in dark theme
- Water and other safe compounds incorrectly showing a Hazards section
- Batch fetch options overlapping the Clear button

---

## [v1.4.0] — 2026-07-01

### Added
- Hazard pictograms and GHS safety data (signal word, hazard statements, precautions) shown per compound
- Batch: opt in to fetching synonyms and hazard data per run

### Fixed
- Compounds with multiple disconnected structures (salts, ionic compounds) rendering only part of the structure
- Names containing parentheses, e.g. "Semax (acetate)", failing to resolve
- Removed unused attribution link from the footer

---

## [v1.3.1] — 2026-06-30

### Fixed
- Wrong hazard classification showing for some compounds
- Hazard pictograms repositioned and enlarged

---

## [v1.3.0] — 2026-06-30

### Added
- Hazard pictograms and a collapsible Hazards section, sourced from ECHA's classification database

---

## [v1.2.2] — 2026-06-30

### Fixed
- Names containing parentheses failing to resolve

---

## [v1.2.1] — 2026-06-30

### Fixed
- Compounds with multiple structures (salts, ionic compounds) rendering incompletely

---

## [v1.2.0] — 2026-06-23

### Added
- "Did you mean?" suggestions when a name lookup fails
- Version number shown in the footer

---

## [v1.1.0] — 2026-06-22

### Added
- Download structures as SVG, PNG, or 3D SDF/XYZ
- Copy structure image to clipboard
- Upload Excel files for batch resolution
- Compact batch results toolbar

---

## [v1.0.0] — 2026-06-17

### Added
- Resolve SMILES, names, CAS numbers, InChIKeys, and InChI via PubChem
- Batch resolution with progress tracking
- Structure preview with copy/download
- Computed properties (LogP, TPSA, H-bond counts, rotatable bonds, and more)
- Synonyms and CAS number lookup
- Dark/light theme
