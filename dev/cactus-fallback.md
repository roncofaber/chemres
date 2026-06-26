# Cactus Fallback Resolver

## What is Cactus?

NCI/CADD Chemical Identifier Resolver, operated by the National Cancer Institute.  
API base: `https://cactus.nci.nih.gov/chemical/structure`

A secondary chemical resolver that can be used as a fallback when PubChem does not recognise an identifier. It accepts the same identifier types we already support and returns a SMILES string which can then be re-submitted to PubChem to retrieve the full compound record.

## API

```
GET https://cactus.nci.nih.gov/chemical/structure/{identifier}/{representation}
```

- Response: plain text, no JSON
- `200` → success, body is the result
- `500` → identifier not found (Cactus uses 500, not 404)
- No authentication required, no documented rate limit

### Tested representations

| representation | notes |
|---|---|
| `smiles` | returns canonical SMILES, URL-safe |
| `inchikey` | returns `InChIKey=XXXX` (prefix must be stripped) |
| `cas` | may return multiple CAS numbers, one per line |

### Tested input types (all work)

| input | example |
|---|---|
| Name | `aspirin` |
| CAS | `50-78-2` |
| InChIKey | `BSYNRYMUTXBXSQ-UHFFFAOYSA-N` |
| InChI | `InChI=1S/C9H8O4/...` (must be URL-encoded) |

**SMILES input**: not useful as a fallback — if PubChem rejected a SMILES, Cactus will also reject it (or return a different compound). Skip Cactus for SMILES inputs.

## Fallback strategy

Trigger: PubChem returns `errNotFound` for any non-SMILES input.

```
AutoResolver.resolve(input):
  1. Try PubChem (as today)
  2. If errNotFound AND input is not SMILES:
     a. GET cactus/.../smiles
     b. If 200 → re-query PubChem with returned SMILES
     c. If PubChem recognises SMILES → return full result
     d. If Cactus 500 or PubChem rejects SMILES → return "Not found"
```

The two-hop (Cactus → SMILES → PubChem) ensures we always return the canonical PubChem record with all properties, CAS, synonyms etc. We never store Cactus data directly.

## Implementation plan

### New file: `internal/resolver/cactus.go`

```go
type cactusClient struct {
    http *http.Client
}

func newCactusClient() *cactusClient {
    return &cactusClient{
        http: &http.Client{Timeout: 10 * time.Second},
    }
}

// smiles returns the SMILES for the given identifier, or "" if not found.
func (c *cactusClient) smiles(ctx context.Context, identifier string) (string, error) {
    u := fmt.Sprintf("https://cactus.nci.nih.gov/chemical/structure/%s/smiles",
        url.PathEscape(identifier))
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
    if err != nil {
        return "", err
    }
    resp, err := c.http.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", nil // 500 = not found, treat as empty
    }
    b, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(b)), nil
}
```

### Changes to `internal/resolver/auto.go`

Add `cactus *cactusClient` to `AutoResolver`. In `resolve()`, after PubChem returns `errNotFound` and the input is not SMILES:

```go
if err == errNotFound && !looksLikeSMILES(input) {
    if smiles, cErr := r.cactus.smiles(ctx, input); cErr == nil && smiles != "" {
        result, err = r.smiles.resolve(ctx, smiles, withSynonyms)
        if err == nil && result.Error == "" {
            result.Input = input // preserve original input
            return result, nil
        }
    }
}
return result, err
```

### Rate limiting

Cactus has no documented limit. Add a conservative shared rate limiter (2 req/s) on `cactusClient` using the same `golang.org/x/time/rate` pattern already in `pubchemClient`. Cactus is only hit on cache misses (PubChem "not found"), so in practice traffic will be very low.

### UI impact

None — the result card and batch table are unchanged. The only visible difference is that compounds previously showing "Not found in PubChem" may now resolve. The result will come from PubChem (via the Cactus-provided SMILES) so all fields populate normally.

### Error handling

- Cactus network error: log WARN, return original "Not found" — don't surface Cactus errors to the user
- Cactus returns SMILES but PubChem rejects it: return "Not found" — the compound may be in Cactus's database but not in PubChem
- Both fail: "Not found in PubChem" as today

## Open questions

- Should the result card indicate that Cactus was used? (e.g. a small "resolved via NCI Cactus" note) Probably not needed for first version.
- Should Cactus fallback apply in batch mode? Yes — same code path.
- Should Cactus have its own rate limiter separate from the PubChem one? Yes, since they are different servers.
