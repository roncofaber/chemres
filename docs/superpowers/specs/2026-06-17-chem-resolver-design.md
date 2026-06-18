# chem-resolver design

Date: 2026-06-17
Repo: github.com/roncofaber/chem-resolver

## Purpose

A small Go + HTMX web app that wraps the PubChem PUG REST API for two-way
chemical name resolution: SMILES → compound info (including IUPAC name) and
name/CAS → compound info (including SMILES). Modelled on loinc-validator but
adapted for chemistry.

## Architecture

```
chem-resolver/
├── main.go
├── go.mod
├── internal/
│   ├── resolver/
│   │   ├── resolver.go     # Resolver interface + CompoundResult type
│   │   ├── smiles.go       # SmilesResolver
│   │   └── name.go         # NameResolver
│   └── handlers/
│       ├── resolve.go      # single-lookup handler
│       ├── batch.go        # batch handler
│       └── templates.go
├── templates/
│   ├── index.html
│   ├── smiles/tab.html
│   ├── name/tab.html
│   └── partials/
│       ├── result.html
│       └── batch_result.html
└── static/
    └── style.css
```

## Core types

```go
type CompoundResult struct {
    Input      string
    CID        int
    IUPAC      string
    Canonical  string
    Isomeric   string
    Formula    string
    MW         string
    InChIKey   string
    CAS        string
    SVG        string    // inlined SVG markup
    ResolvedAt time.Time
    Error      string
}

type Resolver interface {
    SystemID() string
    Name()     string
    Resolve(input string) (CompoundResult, error)
    Batch(inputs []string) ([]CompoundResult, error)
}
```

## PubChem API calls

All calls target `https://pubchem.ncbi.nlm.nih.gov/rest/pug`. No auth required.

**SmilesResolver.Resolve(smiles)**:
1. `GET /compound/smiles/{smiles}/property/IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES/JSON`
   — returns CID + property map.
2. `GET /compound/cid/{cid}/record/SVG?record_type=2d`
   — read body, store as SVG string for inlining.
3. `GET /compound/cid/{cid}/synonyms/JSON`
   — scan synonyms for first match of `^\d+-\d+-\d+$` (CAS number).

**NameResolver.Resolve(name)**:
Same three steps but step 1 uses the `name` namespace:
`GET /compound/name/{name}/property/.../JSON`
Accepts common names, IUPAC names, and CAS numbers — PubChem resolves all.

**Batch**: inputs resolved concurrently via a worker pool bounded to 5 goroutines
(stays within PubChem's rate limit). Results collected in original input order.

## Routing

```
GET  /              → index (tab bar)
POST /smiles/resolve → ResolveHandler (SmilesResolver)
POST /smiles/batch   → BatchHandler (SmilesResolver)
POST /name/resolve   → ResolveHandler (NameResolver)
POST /name/batch     → BatchHandler (NameResolver)
GET  /static/        → static files
```

## UI

Two tabs: "SMILES" and "Name". Each tab has:

- **Single-lookup card**: text input + Resolve button. Result swapped in via
  HTMX below the form.
- **Batch card**: textarea (paste list) + file upload. Results rendered as a
  scrollable table: Input | IUPAC | Formula | MW | CAS | Status.

**Result card layout**:
```
┌──────────────────────────────────────────────┐
│  [SVG ~200×150px, right-floated]             │
│  IUPAC     propan-2-one                      │
│  Formula   C₃H₆O                             │
│  MW        58.08                             │
│  CAS       67-64-1                           │
│  InChIKey  CSCPPACGZOOCGX-...               │
│  SMILES    CC(C)=O                [copy]     │
│  Isomeric  CC(C)=O                [copy]     │
└──────────────────────────────────────────────┘
```

SVG is inlined in the HTMX partial (not an `<img>` tag) so it is CSS-styleable
and requires no separate browser request. Fixed `width`/`height` attributes
prevent layout blowout.

Style is adapted from loinc-validator's `style.css` (same design tokens, same
fonts). Batch "not found" rows highlighted with `--invalid-*` palette variables.

## Error handling

| Condition | Behaviour |
|-----------|-----------|
| PubChem 404 | "Not found" card, no error field |
| PubChem non-200 | Error card: "Could not reach PubChem — please try again." |
| Empty input | Button disabled client-side; 400 + error card server-side |
| SVG fetch failure | Result shown without structure image (non-fatal) |
| Batch row failure | Row marked as error; rest of batch continues |

## Testing

- Unit tests: resolver logic with an injected `*http.Client` pointing to a
  local `httptest.Server`. No real network calls.
- Integration tests: one per resolver, gated with `//go:build integration`,
  hit the real PubChem API.
- Handler tests: `httptest.ResponseRecorder` with a stub resolver.
