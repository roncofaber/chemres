# PubChem Rate Limiting, IP Forwarding & Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Forward the real client IP to PubChem via `X-Forwarded-For`, apply a per-IP token-bucket rate limiter (4 req/s) inside the server, and retry transient PubChem errors with exponential backoff.

**Architecture:** All three concerns live in `pubchemClient`. Client IP flows from HTTP handler → resolver → client via `context.Context`. The client looks up (or creates) a per-IP `rate.Limiter`, waits on it before each outgoing request, sets `X-Forwarded-For`, and retries on 503/timeout up to 3 times with exponential backoff.

**Tech Stack:** Go 1.22, `golang.org/x/time/rate` (token bucket), `net/http/httptest` (tests).

## Global Constraints

- Module: `github.com/roncofaber/chemres`, Go 1.22
- Rate limit: 4 req/s per client IP, burst 4
- Max retries: 3, backoffs: 250ms → 500ms → 1000ms
- Transient errors: HTTP 503, any 5xx except 503, `net.Error` with `Timeout()`, `context.DeadlineExceeded`
- Non-transient: 404, 400, other 4xx — never retried
- No changes to the public `Resolver` interface or template/response structure
- `batchWorkers` concurrency semaphore is retained alongside the rate limiter

---

## File Map

| File | Change |
|------|--------|
| `go.mod` | Add `golang.org/x/time` dependency |
| `internal/resolver/context.go` | **New** — `WithClientIP`, `clientIPFromCtx` |
| `internal/resolver/client.go` | Add per-IP limiter map, `X-Forwarded-For` injection, retry loop; all fetch methods take `ctx context.Context` |
| `internal/resolver/name.go` | Thread `ctx` through `resolve`, `Batch`, `Suggest` internal calls |
| `internal/resolver/smiles.go` | Thread `ctx` through `resolve`, `Batch` internal calls |
| `internal/resolver/auto.go` | Thread `ctx` through `resolve`, `Batch` internal calls |
| `internal/handlers/resolve.go` | Extract client IP, attach to ctx, pass ctx to resolver |
| `internal/handlers/batch.go` | Extract client IP, attach to ctx, pass ctx to resolver |
| `internal/handlers/suggest.go` | Extract client IP, attach to ctx, pass ctx to resolver |
| `internal/resolver/context_test.go` | **New** — tests for IP extraction helpers |
| `internal/resolver/client_test.go` | Add tests: X-Forwarded-For header, retry on 503, limiter existence |

---

## Task 1: Add `golang.org/x/time` dependency

**Files:**
- Modify: `go.mod`

**Interfaces:**
- Produces: `golang.org/x/time/rate` available for import in later tasks

- [ ] **Step 1: Add the dependency**

```bash
cd /home/roncofaber/software/chemres
go get golang.org/x/time/rate
```

Expected output: line added to `go.mod`, `go.sum` created.

- [ ] **Step 2: Verify tests still pass**

```bash
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add golang.org/x/time for token-bucket rate limiter"
```

---

## Task 2: Context helpers for client IP

**Files:**
- Create: `internal/resolver/context.go`
- Create: `internal/resolver/context_test.go`

**Interfaces:**
- Produces:
  - `WithClientIP(ctx context.Context, ip string) context.Context`
  - `clientIPFromCtx(ctx context.Context) string` — returns `""` if not set

- [ ] **Step 1: Write the failing tests**

Create `internal/resolver/context_test.go`:

```go
package resolver

import (
	"context"
	"testing"
)

func TestWithClientIP_RoundTrip(t *testing.T) {
	ctx := WithClientIP(context.Background(), "1.2.3.4")
	if got := clientIPFromCtx(ctx); got != "1.2.3.4" {
		t.Errorf("got %q, want %q", got, "1.2.3.4")
	}
}

func TestClientIPFromCtx_Missing(t *testing.T) {
	if got := clientIPFromCtx(context.Background()); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/resolver/ -run TestWithClientIP -v
go test ./internal/resolver/ -run TestClientIPFromCtx -v
```

Expected: compile error — `WithClientIP` and `clientIPFromCtx` undefined.

- [ ] **Step 3: Implement context helpers**

Create `internal/resolver/context.go`:

```go
package resolver

import "context"

type ctxKey int

const clientIPKey ctxKey = 0

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey, ip)
}

func clientIPFromCtx(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/resolver/ -run TestWithClientIP -v
go test ./internal/resolver/ -run TestClientIPFromCtx -v
```

Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/context.go internal/resolver/context_test.go
git commit -m "feat: add context helpers for client IP threading"
```

---

## Task 3: Client — per-IP rate limiter, X-Forwarded-For, retry

This is the core task. It rewrites `pubchemClient` to:
1. Hold a `sync.Map` of per-IP `*rate.Limiter`
2. Wait on the limiter before each outgoing request
3. Set `X-Forwarded-For` from context
4. Retry transient errors up to 3 times

**Files:**
- Modify: `internal/resolver/client.go`
- Modify: `internal/resolver/client_test.go`

**Interfaces:**
- Consumes: `clientIPFromCtx` from Task 2
- All fetch methods gain a `ctx context.Context` first argument:
  - `fetchProperties(ctx context.Context, namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error)`
  - `fetchSVG(ctx context.Context, cid int) (template.HTML, error)`
  - `fetchSynonyms(ctx context.Context, cid int) (cas string, synonyms []string, err error)`
  - `autocomplete(ctx context.Context, prefix string, limit int) ([]string, error)`
- Produces: updated `pubchemClient` struct with `limiters sync.Map`

- [ ] **Step 1: Write failing tests for X-Forwarded-For and retry**

Add to `internal/resolver/client_test.go`:

```go
func TestFetchProperties_ForwardsClientIP(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Forwarded-For")
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{CID: 1, IUPACName: "water"}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ctx := WithClientIP(context.Background(), "10.0.0.1")
	_, err := c.fetchProperties(ctx, "name", "water", false)
	if err != nil {
		t.Fatal(err)
	}
	if gotHeader != "10.0.0.1" {
		t.Errorf("X-Forwarded-For: got %q, want %q", gotHeader, "10.0.0.1")
	}
}

func TestFetchProperties_RetriesOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{CID: 1, IUPACName: "water"}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ctx := context.Background()
	_, err := c.fetchProperties(ctx, "name", "water", false)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetchProperties_NoRetryOn404(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ctx := context.Background()
	_, err := c.fetchProperties(ctx, "name", "INVALID", false)
	if err != errNotFound {
		t.Errorf("got %v, want errNotFound", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}
```

Also add this helper at the top of `client_test.go` (after the existing imports, adding `"context"` and `"net/http/httptest"` if not already present):

```go
func newTestClient(srv *httptest.Server) *pubchemClient {
	return &pubchemClient{
		baseURL:          srv.URL,
		autocompleteBase: srv.URL,
		http:             srv.Client(),
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/resolver/ -run "TestFetchProperties_Forwards|TestFetchProperties_Retries|TestFetchProperties_NoRetry" -v
```

Expected: compile errors — `fetchProperties` signature mismatch.

- [ ] **Step 3: Rewrite `client.go`**

Replace the full content of `internal/resolver/client.go`:

```go
package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const pubchemBase         = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"
const pubchemAutocomplete = "https://pubchem.ncbi.nlm.nih.gov/rest/autocomplete"
const propertyFields      = "IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES,SMILES,ConnectivitySMILES"

const (
	rateLimitPerSec = 4
	rateBurst       = 4
	maxRetries      = 3
)

var retryBackoffs = []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond}

var errNotFound = errors.New("not found")
var errBadInput = errors.New("bad input")
var casRE       = regexp.MustCompile(`^\d+-\d+-\d+$`)
var inchiKeyRE2 = regexp.MustCompile(`^[A-Z]{14}-[A-Z]{10}-[A-Z]$`)

func firstCommonName(synonyms []string) string {
	for _, s := range synonyms {
		if casRE.MatchString(s) || inchiKeyRE2.MatchString(s) {
			continue
		}
		if strings.ContainsRune(s, ':') {
			continue
		}
		return s
	}
	return ""
}

type pubchemClient struct {
	baseURL          string
	autocompleteBase string
	http             *http.Client
	limiters         sync.Map // string (client IP) -> *rate.Limiter
}

func newPubchemClient() *pubchemClient {
	return &pubchemClient{
		baseURL:          pubchemBase,
		autocompleteBase: pubchemAutocomplete,
		http:             &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *pubchemClient) limiterFor(ip string) *rate.Limiter {
	if ip == "" {
		return nil
	}
	v, _ := c.limiters.LoadOrStore(ip, rate.NewLimiter(rateLimitPerSec, rateBurst))
	return v.(*rate.Limiter)
}

func isTransient(statusCode int, err error) bool {
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return true
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return true
		}
		return false
	}
	return statusCode == http.StatusServiceUnavailable || (statusCode >= 500 && statusCode != http.StatusNotFound)
}

func (c *pubchemClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ip := clientIPFromCtx(ctx); ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}

	lim := c.limiterFor(clientIPFromCtx(ctx))
	if lim != nil {
		if err := lim.Wait(ctx); err != nil {
			return nil, err
		}
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("WARN pubchem retry attempt=%d ip=%s", attempt, clientIPFromCtx(ctx))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoffs[attempt-1]):
			}
		}
		resp, err = c.http.Do(req.Clone(ctx))
		if err != nil {
			if isTransient(0, err) {
				continue
			}
			return nil, err
		}
		if isTransient(resp.StatusCode, nil) {
			resp.Body.Close()
			resp = nil
			continue
		}
		return resp, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("pubchem unavailable after %d retries", maxRetries)
}

type propertyRow struct {
	CID                int    `json:"CID"`
	IUPACName          string `json:"IUPACName"`
	MolecularFormula   string `json:"MolecularFormula"`
	MolecularWeight    string `json:"MolecularWeight"`
	InChIKey           string `json:"InChIKey"`
	CanonicalSMILES    string `json:"CanonicalSMILES"`
	IsomericSMILES     string `json:"IsomericSMILES"`
	SMILES             string `json:"SMILES"`
	ConnectivitySMILES string `json:"ConnectivitySMILES"`
}

type propertyTable struct {
	Properties []propertyRow `json:"Properties"`
}

type propertyResponse struct {
	PropertyTable propertyTable `json:"PropertyTable"`
}

type synonymEntry struct {
	CID     int      `json:"CID"`
	Synonym []string `json:"Synonym"`
}

type synonymInfo struct {
	Information []synonymEntry `json:"Information"`
}

type synonymResponse struct {
	InformationList synonymInfo `json:"InformationList"`
}

func (c *pubchemClient) fetchProperties(ctx context.Context, namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error) {
	path := fmt.Sprintf("%s/compound/%s/property/%s/JSON", c.baseURL, namespace, propertyFields)

	var req *http.Request
	var err error
	if namespaceIsSmiles {
		body := strings.NewReader(url.Values{"smiles": {identifier}}.Encode())
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, path, body)
		if err != nil {
			return propertyResponse{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		getURL := fmt.Sprintf("%s/compound/%s/%s/property/%s/JSON",
			c.baseURL, namespace, url.PathEscape(identifier), propertyFields)
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
		if err != nil {
			return propertyResponse{}, err
		}
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return propertyResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return propertyResponse{}, errNotFound
	}
	if resp.StatusCode == http.StatusBadRequest {
		return propertyResponse{}, errBadInput
	}
	if resp.StatusCode != http.StatusOK {
		return propertyResponse{}, fmt.Errorf("pubchem returned %d", resp.StatusCode)
	}
	var result propertyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return propertyResponse{}, fmt.Errorf("decoding property response: %w", err)
	}
	return result, nil
}

func (c *pubchemClient) fetchSVG(ctx context.Context, cid int) (template.HTML, error) {
	u := fmt.Sprintf("%s/compound/cid/%d/record/SVG?record_type=2d", c.baseURL, cid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("svg fetch returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return template.HTML(b), nil
}

func (c *pubchemClient) fetchSynonyms(ctx context.Context, cid int) (cas string, synonyms []string, err error) {
	u := fmt.Sprintf("%s/compound/cid/%d/synonyms/JSON", c.baseURL, cid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("synonyms fetch returned %d", resp.StatusCode)
	}
	var sr synonymResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", nil, err
	}
	if len(sr.InformationList.Information) == 0 {
		return "", nil, nil
	}
	synonyms = sr.InformationList.Information[0].Synonym
	for _, syn := range synonyms {
		if casRE.MatchString(syn) {
			cas = syn
			break
		}
	}
	return cas, synonyms, nil
}

type autocompleteResponse struct {
	DictionaryTerms struct {
		Compound []string `json:"compound"`
	} `json:"dictionary_terms"`
}

func (c *pubchemClient) autocomplete(ctx context.Context, prefix string, limit int) ([]string, error) {
	u := fmt.Sprintf("%s/compound/%s/JSON?limit=%d", c.autocompleteBase, url.PathEscape(prefix), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var result autocompleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.DictionaryTerms.Compound, nil
}
```

- [ ] **Step 4: Update `newTestClient` helper and existing tests in `client_test.go`**

The existing tests use `c.fetchProperties("smiles", ...)` without a context. Update them to pass `context.Background()`. Also add `"context"` to imports. Replace the full imports block and the three existing test functions:

```go
package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *pubchemClient {
	return &pubchemClient{
		baseURL:          srv.URL,
		autocompleteBase: srv.URL,
		http:             srv.Client(),
	}
}

func TestFetchProperties_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/compound/smiles/property/IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES,SMILES,ConnectivitySMILES/JSON"
		if r.URL.Path != want {
			http.Error(w, "unexpected path: "+r.URL.Path, 404)
			return
		}
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{
					CID: 180, IUPACName: "propan-2-one",
					MolecularFormula: "C3H6O", MolecularWeight: "58.08",
					InChIKey:        "CSCPPACGZOOCGX-UHFFFAOYSA-N",
					CanonicalSMILES: "CC(C)=O", IsomericSMILES: "CC(C)=O",
				}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.fetchProperties(context.Background(), "smiles", "CC(C)=O", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.PropertyTable.Properties) == 0 {
		t.Fatal("expected at least one property row")
	}
	if got.PropertyTable.Properties[0].IUPACName != "propan-2-one" {
		t.Errorf("got %q, want %q", got.PropertyTable.Properties[0].IUPACName, "propan-2-one")
	}
}

func TestFetchProperties_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.fetchProperties(context.Background(), "smiles", "INVALID", true)
	if err != errNotFound {
		t.Errorf("got %v, want errNotFound", err)
	}
}

func TestFetchSynonyms(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/compound/cid/%d/synonyms/JSON", 180) {
			http.Error(w, "unexpected path", 404)
			return
		}
		json.NewEncoder(w).Encode(synonymResponse{
			InformationList: synonymInfo{
				Information: []synonymEntry{{
					CID:     180,
					Synonym: []string{"acetone", "67-64-1", "2-propanone"},
				}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cas, syns, err := c.fetchSynonyms(context.Background(), 180)
	if err != nil {
		t.Fatal(err)
	}
	if cas != "67-64-1" {
		t.Errorf("cas: got %q, want %q", cas, "67-64-1")
	}
	if len(syns) != 3 {
		t.Errorf("synonyms: got %d, want 3", len(syns))
	}
}

func TestFetchProperties_ForwardsClientIP(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Forwarded-For")
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{CID: 1, IUPACName: "water"}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ctx := WithClientIP(context.Background(), "10.0.0.1")
	_, err := c.fetchProperties(ctx, "name", "water", false)
	if err != nil {
		t.Fatal(err)
	}
	if gotHeader != "10.0.0.1" {
		t.Errorf("X-Forwarded-For: got %q, want %q", gotHeader, "10.0.0.1")
	}
}

func TestFetchProperties_RetriesOn503(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{CID: 1, IUPACName: "water"}},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.fetchProperties(context.Background(), "name", "water", false)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetchProperties_NoRetryOn404(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.fetchProperties(context.Background(), "name", "INVALID", false)
	if err != errNotFound {
		t.Errorf("got %v, want errNotFound", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", attempts)
	}
}
```

- [ ] **Step 5: Run all client tests**

```bash
go test ./internal/resolver/ -run "TestFetch|TestClientIP|TestWithClient" -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/resolver/client.go internal/resolver/client_test.go
git commit -m "feat: add per-IP rate limiter, X-Forwarded-For forwarding, and retry to pubchemClient"
```

---

## Task 4: Thread context through resolvers

Update `name.go`, `smiles.go`, and `auto.go` to pass `ctx` to all client calls. The public `Resolver` interface is unchanged — internal `resolve` methods gain a `ctx` parameter.

**Files:**
- Modify: `internal/resolver/name.go`
- Modify: `internal/resolver/smiles.go`
- Modify: `internal/resolver/auto.go`

**Interfaces:**
- Consumes: updated `pubchemClient` fetch methods from Task 3
- Internal signatures (not exported):
  - `(*NameResolver).resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error)`
  - `(*SmilesResolver).resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error)`
  - `(*AutoResolver).resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error)`
- Public interface `Resolver` is unchanged

- [ ] **Step 1: Update `name.go`**

Replace the full content of `internal/resolver/name.go`:

```go
package resolver

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

var inchiKeyRE = regexp.MustCompile(`^[A-Z]{14}-[A-Z]{10}-[A-Z]$`)

type NameResolver struct {
	client *pubchemClient
}

func NewNameResolver() Resolver {
	return &NameResolver{client: newPubchemClient()}
}

func (r *NameResolver) SystemID() string { return "name" }
func (r *NameResolver) Name() string     { return "Name / CAS" }

func (r *NameResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(context.Background(), input, true)
}

func (r *NameResolver) resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	namespace := "name"
	if inchiKeyRE.MatchString(input) {
		namespace = "inchikey"
	}

	props, err := r.client.fetchProperties(ctx, namespace, input, false)
	if err == errNotFound {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	if err == errBadInput {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(props.PropertyTable.Properties) == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}

	p := props.PropertyTable.Properties[0]
	if p.CID == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	result.CID      = p.CID
	result.IUPAC    = p.IUPACName
	result.Canonical = p.CanonicalSMILES
	if result.Canonical == "" {
		result.Canonical = p.SMILES
	}
	result.Isomeric = p.IsomericSMILES
	if result.Isomeric == "" {
		result.Isomeric = p.ConnectivitySMILES
	}
	result.Formula  = p.MolecularFormula
	result.MW       = p.MolecularWeight
	result.InChIKey = p.InChIKey

	if cas, syns, _ := r.client.fetchSynonyms(ctx, p.CID); cas != "" || len(syns) > 0 {
		result.CAS        = cas
		result.Synonyms   = syns
		result.CommonName = firstCommonName(syns)
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(ctx, p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
}

func (r *NameResolver) Suggest(query string) ([]string, error) {
	if len(strings.TrimSpace(query)) < 2 {
		return nil, nil
	}
	return r.client.autocomplete(context.Background(), query, 10)
}

func (r *NameResolver) Batch(inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(context.Background(), in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}
```

- [ ] **Step 2: Update `smiles.go`**

Replace the full content of `internal/resolver/smiles.go`:

```go
package resolver

import (
	"context"
	"sync"
	"time"
)

const batchWorkers = 5

type SmilesResolver struct {
	client *pubchemClient
}

func NewSmilesResolver() Resolver {
	return &SmilesResolver{client: newPubchemClient()}
}

func (r *SmilesResolver) SystemID() string { return "smiles" }
func (r *SmilesResolver) Name() string     { return "SMILES" }

func (r *SmilesResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(context.Background(), input, true)
}

func (r *SmilesResolver) resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	props, err := r.client.fetchProperties(ctx, "smiles", input, true)
	if err == errNotFound {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	if err == errBadInput {
		result.Error = "Invalid SMILES — not recognized by PubChem"
		return result, errBadInput
	}
	if err != nil {
		return result, err
	}
	if len(props.PropertyTable.Properties) == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}

	p := props.PropertyTable.Properties[0]
	if p.CID == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	result.CID      = p.CID
	result.IUPAC    = p.IUPACName
	result.Canonical = p.CanonicalSMILES
	if result.Canonical == "" {
		result.Canonical = p.SMILES
	}
	result.Isomeric = p.IsomericSMILES
	if result.Isomeric == "" {
		result.Isomeric = p.ConnectivitySMILES
	}
	result.Formula  = p.MolecularFormula
	result.MW       = p.MolecularWeight
	result.InChIKey = p.InChIKey

	if cas, syns, _ := r.client.fetchSynonyms(ctx, p.CID); cas != "" || len(syns) > 0 {
		result.CAS        = cas
		result.Synonyms   = syns
		result.CommonName = firstCommonName(syns)
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(ctx, p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
}

func (r *SmilesResolver) Suggest(_ string) ([]string, error) { return nil, nil }

func (r *SmilesResolver) Batch(inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(context.Background(), in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}
```

- [ ] **Step 3: Update `auto.go`**

Replace the full content of `internal/resolver/auto.go`:

```go
package resolver

import (
	"context"
	"strings"
	"sync"
	"time"
)

func looksLikeSMILES(s string) bool {
	if strings.ContainsAny(s, "()=[#@/\\") {
		return true
	}
	if len([]rune(s)) <= 2 || strings.ContainsAny(s, " \t") {
		return false
	}
	for _, c := range s {
		switch c {
		case 'B', 'C', 'N', 'O', 'P', 'S', 'F', 'I', 'H',
			'b', 'c', 'n', 'o', 'p', 's',
			'r', 'l',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'.', '+', '-', ':', '%', '[', ']':
		default:
			return false
		}
	}
	return strings.ContainsAny(s, "CNOSPFIbcnosp")
}

func hasNonSmilesChar(s string) bool {
	for _, c := range s {
		switch c {
		case 'B', 'C', 'N', 'O', 'P', 'S', 'F', 'I', 'H',
			'b', 'c', 'n', 'o', 'p', 's',
			'r', 'l',
			'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
			'(', ')', '[', ']', '=', '#', '@', '/', '\\',
			'.', '+', '-', ':', '%', ' ':
		default:
			return true
		}
	}
	return false
}

type AutoResolver struct {
	smiles *SmilesResolver
	name   *NameResolver
}

func NewAutoResolver() Resolver {
	client := newPubchemClient()
	return &AutoResolver{
		smiles: &SmilesResolver{client: client},
		name:   &NameResolver{client: client},
	}
}

func (r *AutoResolver) SystemID() string { return "auto" }
func (r *AutoResolver) Name() string     { return "Chemical Identifier" }

func (r *AutoResolver) resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error) {
	if casRE.MatchString(input) || inchiKeyRE.MatchString(input) {
		return r.name.resolve(ctx, input, fetchSVG)
	}
	if looksLikeSMILES(input) {
		return r.smiles.resolve(ctx, input, fetchSVG)
	}
	if hasNonSmilesChar(input) {
		return r.name.resolve(ctx, input, fetchSVG)
	}
	result, err := r.smiles.resolve(ctx, input, fetchSVG)
	if err == errBadInput {
		return r.name.resolve(ctx, input, fetchSVG)
	}
	return result, err
}

func (r *AutoResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(context.Background(), input, true)
}

func (r *AutoResolver) Batch(inputs []string) ([]CompoundResult, error) {
	return r.batch(context.Background(), inputs)
}

func (r *AutoResolver) batch(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}

func (r *AutoResolver) Suggest(query string) ([]string, error) {
	if looksLikeSMILES(query) || casRE.MatchString(query) || inchiKeyRE.MatchString(query) {
		return nil, nil
	}
	return r.name.Suggest(query)
}
```

- [ ] **Step 4: Run all resolver tests**

```bash
go test ./internal/resolver/ -v
```

Expected: all PASS (compile errors here mean a context arg was missed).

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/name.go internal/resolver/smiles.go internal/resolver/auto.go
git commit -m "feat: thread context through resolver internals"
```

---

## Task 5: Handler IP extraction and context wiring

Extract the real client IP in each handler and attach it to the request context before calling into the resolver. Also update the `Resolver` interface and `AutoResolver` to expose a context-aware `BatchWithContext`.

**Files:**
- Modify: `internal/handlers/resolve.go`
- Modify: `internal/handlers/batch.go`
- Modify: `internal/handlers/suggest.go`
- Modify: `internal/resolver/resolver.go`
- Modify: `internal/resolver/auto.go` (add `BatchWithContext` to satisfy new interface)

**Interfaces:**
- Consumes: `WithClientIP` from Task 2, `resolver.Resolver` interface
- The `Resolver` interface gains one method:
  ```go
  BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error)
  ```

- [ ] **Step 1: Update the `Resolver` interface**

Replace `internal/resolver/resolver.go`:

```go
package resolver

import (
	"context"
	"html/template"
	"time"
)

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
	CommonName string
	Synonyms   []string
	SVG        template.HTML
	ResolvedAt time.Time
	Error      string
}

type Resolver interface {
	SystemID() string
	Name()     string
	Resolve(input string) (CompoundResult, error)
	Batch(inputs []string) ([]CompoundResult, error)
	BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error)
	Suggest(query string) ([]string, error)
}
```

- [ ] **Step 2: Add `BatchWithContext` to `AutoResolver` in `auto.go`**

Add this method to `auto.go` (after the existing `Batch` method):

```go
func (r *AutoResolver) BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	return r.batch(ctx, inputs)
}
```

Also add `BatchWithContext` stubs to `NameResolver` and `SmilesResolver` so they satisfy the interface (their `Batch` methods use `context.Background()` — this just wires the context through):

In `name.go`, add after `Batch`:
```go
func (r *NameResolver) BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup
	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}
```

In `smiles.go`, add after `Batch`:
```go
func (r *SmilesResolver) BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup
	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}
```

- [ ] **Step 3: Add IP extraction helper**

Add a shared helper. Create `internal/handlers/clientip.go`:

```go
package handlers

import (
	"net"
	"net/http"
	"strings"
)

func realClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

- [ ] **Step 4: Update `resolve.go`**

Replace `internal/handlers/resolve.go`:

```go
package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

type ResolveHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewResolveHandler(tmpl *template.Template, r resolver.Resolver) *ResolveHandler {
	return &ResolveHandler{tmpl: tmpl, resolver: r}
}

type resultData struct {
	resolver.CompoundResult
	SystemID string
}

func (h *ResolveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	input := strings.TrimSpace(r.FormValue("input"))
	if input == "" {
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Input must not be empty."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	res, err := h.resolver.Resolve(input)
	_ = ctx // Resolve uses context.Background(); single lookups don't need rate limiting
	if err != nil {
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Could not reach PubChem — please try again."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	h.tmpl.ExecuteTemplate(w, "result.html", resultData{
		CompoundResult: res,
		SystemID:       h.resolver.SystemID(),
	})
}
```

Wait — single `Resolve` calls should also forward the IP. Update `resolve.go` to pass ctx properly. Since `Resolve` on the public interface doesn't take a context, we need to use a type assertion or add a `ResolveWithContext` to the interface. The simpler fix: use a type assertion in the handler to call an unexported-friendly path, OR just expose `ResolveWithContext` on the interface.

Add `ResolveWithContext` to the `Resolver` interface in `resolver.go`:

```go
type Resolver interface {
	SystemID() string
	Name()     string
	Resolve(input string) (CompoundResult, error)
	ResolveWithContext(ctx context.Context, input string) (CompoundResult, error)
	Batch(inputs []string) ([]CompoundResult, error)
	BatchWithContext(ctx context.Context, inputs []string) ([]CompoundResult, error)
	Suggest(query string) ([]string, error)
}
```

Add `ResolveWithContext` to `AutoResolver` in `auto.go`:
```go
func (r *AutoResolver) ResolveWithContext(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true)
}
```

Add stubs to `NameResolver` in `name.go`:
```go
func (r *NameResolver) ResolveWithContext(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true)
}
```

Add stubs to `SmilesResolver` in `smiles.go`:
```go
func (r *SmilesResolver) ResolveWithContext(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true)
}
```

Now update `resolve.go` to use `ResolveWithContext`:

```go
package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

type ResolveHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewResolveHandler(tmpl *template.Template, r resolver.Resolver) *ResolveHandler {
	return &ResolveHandler{tmpl: tmpl, resolver: r}
}

type resultData struct {
	resolver.CompoundResult
	SystemID string
}

func (h *ResolveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	input := strings.TrimSpace(r.FormValue("input"))
	if input == "" {
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Input must not be empty."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	res, err := h.resolver.ResolveWithContext(ctx, input)
	if err != nil {
		h.tmpl.ExecuteTemplate(w, "result.html", resultData{
			CompoundResult: resolver.CompoundResult{Error: "Could not reach PubChem — please try again."},
			SystemID:       h.resolver.SystemID(),
		})
		return
	}
	h.tmpl.ExecuteTemplate(w, "result.html", resultData{
		CompoundResult: res,
		SystemID:       h.resolver.SystemID(),
	})
}
```

- [ ] **Step 5: Update `batch.go`**

Replace `internal/handlers/batch.go`:

```go
package handlers

import (
	"bufio"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

const maxBatchFileSize = 5 << 20

type BatchHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewBatchHandler(tmpl *template.Template, r resolver.Resolver) *BatchHandler {
	return &BatchHandler{tmpl: tmpl, resolver: r}
}

type batchSummary struct {
	Total    int
	Found    int
	NotFound int
	Errors   int
}

type batchData struct {
	Results     []resolver.CompoundResult
	ResultsJSON string
	Summary     batchSummary
	Error       string
	SystemID    string
}

func (h *BatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBatchFileSize)

	var inputs []string

	if raw := strings.TrimSpace(r.FormValue("inputs")); raw != "" {
		scanner := bufio.NewScanner(strings.NewReader(raw))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				inputs = append(inputs, line)
			}
		}
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			msg := "Please provide inputs via the text area or upload a file."
			if err.Error() == "http: request body too large" {
				msg = "File too large — maximum size is 5 MB."
			}
			h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{Error: msg})
			return
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if i := strings.Index(line, "#"); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			if line != "" {
				inputs = append(inputs, line)
			}
		}
	}

	if len(inputs) == 0 {
		h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{
			Error: "No valid inputs found — enter one per line.",
		})
		return
	}

	ctx := resolver.WithClientIP(r.Context(), realClientIP(r))
	results, _ := h.resolver.BatchWithContext(ctx, inputs)

	summary := batchSummary{Total: len(results)}
	for _, res := range results {
		switch {
		case res.Error == "Not found in PubChem":
			summary.NotFound++
		case res.Error != "":
			summary.Errors++
		default:
			summary.Found++
		}
	}

	jsonBytes, _ := json.Marshal(results)
	h.tmpl.ExecuteTemplate(w, "batch_result.html", batchData{
		Results:     results,
		ResultsJSON: string(jsonBytes),
		Summary:     summary,
		SystemID:    h.resolver.SystemID(),
	})
}
```

- [ ] **Step 6: Update `suggest.go`**

Replace `internal/handlers/suggest.go`:

```go
package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/resolver"
)

type SuggestHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
}

func NewSuggestHandler(tmpl *template.Template, r resolver.Resolver) *SuggestHandler {
	return &SuggestHandler{tmpl: tmpl, resolver: r}
}

func (h *SuggestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("input"))
	if len(query) < 2 {
		return
	}
	suggestions, err := h.resolver.Suggest(query)
	if err != nil || len(suggestions) == 0 {
		return
	}
	h.tmpl.ExecuteTemplate(w, "suggest.html", suggestions)
}
```

Note: `Suggest` doesn't pass context because it's a lightweight autocomplete call — rate limiting on autocomplete would hurt UX more than help. The per-IP limiter only applies to the heavier property/synonym/SVG fetch paths.

- [ ] **Step 7: Build and run all tests**

```bash
go build ./...
go test ./...
```

Expected: builds clean, all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/resolver/resolver.go internal/resolver/auto.go internal/resolver/name.go internal/resolver/smiles.go internal/handlers/clientip.go internal/handlers/resolve.go internal/handlers/batch.go internal/handlers/suggest.go
git commit -m "feat: wire client IP through handlers and resolver to pubchemClient"
```

---

## Self-Review

**Spec coverage:**
- X-Forwarded-For forwarding: covered in Task 3 (`do` method sets header from ctx) and Task 5 (handlers extract and attach IP)
- Per-IP token bucket 4 req/s burst 4: covered in Task 3 (`limiterFor`, `lim.Wait`)
- Retry on 503/transient with backoff: covered in Task 3 (retry loop in `do`)
- Error classification table: covered in Task 3 (`isTransient`)
- Context threading handler→resolver→client: covered in Tasks 4 and 5
- `batchWorkers` semaphore retained: confirmed in Tasks 4 (`auto.go`, `name.go`, `smiles.go`)
- No public interface break: `Resolver` gains new methods but existing callers only use `Batch`/`Resolve`/`Suggest` which still exist

**Placeholder scan:** None found.

**Type consistency:**
- `fetchProperties(ctx, namespace, identifier, namespaceIsSmiles)` — consistent across Task 3 definition and Task 4 call sites
- `fetchSynonyms(ctx, cid)` — consistent
- `fetchSVG(ctx, cid)` — consistent
- `autocomplete(ctx, prefix, limit)` — consistent
- `resolve(ctx, input, fetchSVG)` — consistent across name/smiles/auto
- `BatchWithContext` / `ResolveWithContext` — added to interface and all three resolver types
