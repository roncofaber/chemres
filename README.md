# Chem Resolver

A lightweight web app for resolving chemical identifiers via the [PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/).

Accepts **SMILES**, **names**, **CAS numbers**, and **InChIKeys** — type any of them in the single input field and get back the compound's IUPAC name, molecular formula, molecular weight, canonical SMILES, InChIKey, CAS number, 2D structure, and synonyms.

## Features

- **Auto-detection** — no need to select input type; SMILES, names, CAS numbers, and InChIKeys are routed automatically
- **Single lookup** with autocomplete for names
- **Batch mode** — paste a list or upload a file; results exported as CSV
- **2D structure rendering** — client-side SVG via SmilesDrawer; click to enlarge, download as SVG
- **Dark / light theme** — persisted per browser
- **No dependencies** — single Go binary + templates; zero external packages

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

The app is a single stateless Go binary. Any platform that runs Docker containers works (Fly.io, Railway, Render, Cloud Run, etc.).

Set the `PORT` environment variable if the platform injects it (most do).

## Attribution

- **[PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/)** — compound data, properties, and synonyms (National Library of Medicine / NIH)
- **[SmilesDrawer](https://github.com/reymond-group/smilesDrawer)** — client-side 2D structure rendering  
  Probst, D.; Reymond, J.-L. *SmilesDrawer: Parsing and Drawing SMILES-Encoded Molecular Structures Using Client-Side JavaScript.* J. Chem. Inf. Model. **2018**, 58, 1–7. DOI: [10.1021/acs.jcim.7b00425](https://doi.org/10.1021/acs.jcim.7b00425)
