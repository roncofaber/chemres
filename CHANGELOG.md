# Changelog

## [Unreleased]

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
