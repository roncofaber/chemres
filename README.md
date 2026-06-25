# Chemical Resolver

A lightweight web app for resolving chemical identifiers via the [PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/).

**Live:** [chemres.app](https://chemres.app)

Accepts **SMILES**, **names**, **CAS numbers**, and **InChIKeys** — type any of them and get back the compound's preferred name, IUPAC name, molecular formula, molecular weight, canonical SMILES, InChIKey, CAS number, 2D structure, and synonyms.

## Features

- **Auto-detection** — SMILES, names, CAS numbers, and InChIKeys are routed automatically; no need to select input type
- **Autocomplete** — name suggestions with keyboard navigation as you type
- **Single lookup** — instant result with structure drawing, copyable fields, and a direct PubChem link
- **Batch mode** — paste a list or upload a plain text file; real-time progress bar via SSE; results in an interactive table with sortable columns and per-row expand cards showing structure + full details
- **Retry** — re-resolve individual failed batch entries without re-running the whole batch
- **CSV export** — includes PubChem URL, common name, InChIKey, SMILES, and fetch timestamp for reproducibility
- **2D structure rendering** — client-side via SmilesDrawer; theme-aware (light/dark); click to enlarge, download as SVG or copy as PNG
- **Dark / light theme** — persisted per browser; structures re-render on toggle
- **Mobile responsive** — usable on phones; batch table adapts to narrow screens
- **Rate limiting** — per-user token bucket (4 req/s) via `X-Forwarded-For` forwarding; requests retry automatically on transient errors

## Running locally

```sh
go run .
```

Opens on [http://localhost:8080](http://localhost:8080). Set `PORT` to change the port.

## Running with Docker

```sh
docker build -t chemres .
docker run -p 8080:8080 chemres
```

## Deployment

The app is a stateless Go binary. Any platform that runs Docker containers works (Fly.io, Railway, Render, Cloud Run, etc.).

Set the `PORT` environment variable if the platform injects it (most do). The included `fly.toml` targets Fly.io with auto-stop/start to keep costs near zero for light usage.

## Attribution

- **[PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/)** — compound data, properties, and synonyms (National Library of Medicine / NIH)
- **[SmilesDrawer](https://github.com/reymond-group/smilesDrawer)** — client-side 2D structure rendering  
  Probst, D.; Reymond, J.-L. *SmilesDrawer: Parsing and Drawing SMILES-Encoded Molecular Structures Using Client-Side JavaScript.* J. Chem. Inf. Model. **2018**, 58, 1–7. DOI: [10.1021/acs.jcim.7b00425](https://doi.org/10.1021/acs.jcim.7b00425)
