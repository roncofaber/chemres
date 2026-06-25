# Chemical Resolver

A lightweight web app for resolving chemical identifiers via the [PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/).

**Live:** [chemres.app](https://chemres.app)

Accepts **SMILES**, **names**, **CAS numbers**, and **InChIKeys**. Type any of them and get back the compound's preferred name, IUPAC name, molecular formula, molecular weight, canonical SMILES, InChIKey, CAS number, 2D structure, and synonyms.

## Features

- **Auto-detection**: SMILES, names, CAS numbers, and InChIKeys are routed automatically
- **Autocomplete**: name suggestions with keyboard navigation as you type
- **Batch mode**: paste a list or upload a plain text file to get results in an interactive table
- **Retry**: re-resolve individual failed batch entries without re-running the whole batch
- **CSV export**: export batch results to a file
- **2D structure rendering**: chemical structures rendered using SmilesDrawer can be downloaded as SVG or copied as PNG

## API

A JSON API is available at `https://chemres.app/api/v1/`. All endpoints support CORS.

### Resolve a single compound

```bash
curl -X POST https://chemres.app/api/v1/resolve \
  -H "Content-Type: application/json" \
  -d '{"input": "acetone"}'
```

### Batch resolve (up to 500 compounds)

```bash
curl -X POST https://chemres.app/api/v1/batch \
  -H "Content-Type: application/json" \
  -d '{"inputs": ["acetone", "water", "CCO"]}'
```

### Autocomplete

```bash
curl "https://chemres.app/api/v1/suggest?q=acet"
```

Responses use snake_case JSON fields (`input`, `cid`, `iupac`, `formula`, `mw`, `cas`, `inchikey`, `canonical_smiles`, `isomeric_smiles`, `common_name`, `synonyms`, `resolved_at`, `error`). Inputs are limited to 5000 characters each. Requests are rate-limited to 4 req/s per IP via the PubChem API.

## Deployment

A live version of the app is available at [chemres.app](https://chemres.app). The app can also be run locally as a stateless Go binary. Run it locally with Go, Docker or deploy it on any platform that runs Docker containers.

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

## Attribution

- **[PubChem REST API](https://pubchem.ncbi.nlm.nih.gov/)**: compound data, properties, and synonyms (National Library of Medicine / NIH)
- **[SmilesDrawer](https://github.com/reymond-group/smilesDrawer)**: 2D structure rendering [^1]

[^1]: Probst, D & Reymond, J.-L. (2017). *SmilesDrawer: Parsing and Drawing SMILES-Encoded Molecular Structures Using Client-Side JavaScript.* Journal of Chemical Information and Modeling. 58. DOI: [10.1021/acs.jcim.7b00425](https://doi.org/10.1021/acs.jcim.7b00425)