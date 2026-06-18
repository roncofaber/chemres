# chem-resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go + HTMX web app that wraps the PubChem REST API to resolve SMILES → compound info and name/CAS → compound info, with single-lookup and batch modes.

**Architecture:** Two `Resolver` implementations (`SmilesResolver`, `NameResolver`) share a PubChem HTTP client. Generic `ResolveHandler` and `BatchHandler` serve both. HTMX swaps partial HTML into the page without full reloads. SVG structure images are fetched server-side and inlined.

**Tech Stack:** Go 1.22+, standard library only (no external dependencies), HTMX 2.0.4 via CDN, PubChem PUG REST API.

## Global Constraints

- Module path: `github.com/roncofaber/chem-resolver`
- Go 1.22+, zero external dependencies (stdlib only)
- PubChem batch worker pool capped at 5 concurrent goroutines
- `CompoundResult.SVG` is `template.HTML` to allow safe inline rendering
- SMILES property lookups use HTTP POST (special chars break URL paths)
- Name/CAS lookups use HTTP GET with `url.PathEscape`
- Batch skips SVG fetching; single-lookup fetches SVG (failure is non-fatal)
- Integration tests gated with `//go:build integration`

---

### Task 1: Module scaffold + Resolver interface

**Files:**
- Create: `go.mod`
- Create: `internal/resolver/resolver.go`

**Interfaces:**
- Produces: `CompoundResult`, `Resolver` — used by all subsequent tasks

- [ ] **Step 1: Write `go.mod`**

```
module github.com/roncofaber/chem-resolver

go 1.22
```

- [ ] **Step 2: Write `internal/resolver/resolver.go`**

```go
package resolver

import (
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
	SVG        template.HTML
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

- [ ] **Step 3: Verify it compiles**

```bash
cd /home/roncofaber/software/chem-resolver
go build ./...
```
Expected: no output, exit 0.

- [ ] **Step 4: Commit**

```bash
git add go.mod internal/resolver/resolver.go
git commit -m "feat: scaffold module and Resolver interface"
```

---

### Task 2: Shared PubChem HTTP client

**Files:**
- Create: `internal/resolver/client.go`
- Create: `internal/resolver/client_test.go`

**Interfaces:**
- Produces: `pubchemClient` with:
  - `fetchProperties(namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error)`
  - `fetchSVG(cid int) (template.HTML, error)`
  - `fetchCAS(cid int) (string, error)`
  - `errNotFound` sentinel error

- [ ] **Step 1: Write the failing test**

```go
// internal/resolver/client_test.go
package resolver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchProperties_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/compound/smiles/property/IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES/JSON"
		if r.URL.Path != want {
			http.Error(w, "unexpected path: "+r.URL.Path, 404)
			return
		}
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{
					CID: 180, IUPACName: "propan-2-one",
					MolecularFormula: "C3H6O", MolecularWeight: "58.08",
					InChIKey: "CSCPPACGZOOCGX-UHFFFAOYSA-N",
					CanonicalSMILES: "CC(C)=O", IsomericSMILES: "CC(C)=O",
				}},
			},
		})
	}))
	defer srv.Close()

	c := &pubchemClient{baseURL: srv.URL, http: srv.Client()}
	got, err := c.fetchProperties("smiles", "CC(C)=O", true)
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

	c := &pubchemClient{baseURL: srv.URL, http: srv.Client()}
	_, err := c.fetchProperties("smiles", "INVALID", true)
	if err != errNotFound {
		t.Errorf("got %v, want errNotFound", err)
	}
}

func TestFetchCAS(t *testing.T) {
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

	c := &pubchemClient{baseURL: srv.URL, http: srv.Client()}
	cas, err := c.fetchCAS(180)
	if err != nil {
		t.Fatal(err)
	}
	if cas != "67-64-1" {
		t.Errorf("got %q, want %q", cas, "67-64-1")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -run TestFetch -v
```
Expected: compilation error (types not defined yet).

- [ ] **Step 3: Write `internal/resolver/client.go`**

```go
package resolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const pubchemBase = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"
const propertyFields = "IUPACName,MolecularFormula,MolecularWeight,InChIKey,CanonicalSMILES,IsomericSMILES"

var errNotFound = errors.New("not found")
var casRE = regexp.MustCompile(`^\d+-\d+-\d+$`)

type pubchemClient struct {
	baseURL string
	http    *http.Client
}

func newPubchemClient() *pubchemClient {
	return &pubchemClient{
		baseURL: pubchemBase,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type propertyRow struct {
	CID              int    `json:"CID"`
	IUPACName        string `json:"IUPACName"`
	MolecularFormula string `json:"MolecularFormula"`
	MolecularWeight  string `json:"MolecularWeight"`
	InChIKey         string `json:"InChIKey"`
	CanonicalSMILES  string `json:"CanonicalSMILES"`
	IsomericSMILES   string `json:"IsomericSMILES"`
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

// fetchProperties calls PubChem for compound properties.
// SMILES uses POST to handle special characters; name/CAS uses GET.
func (c *pubchemClient) fetchProperties(namespace, identifier string, namespaceIsSmiles bool) (propertyResponse, error) {
	path := fmt.Sprintf("%s/compound/%s/property/%s/JSON", c.baseURL, namespace, propertyFields)

	var (
		resp *http.Response
		err  error
	)
	if namespaceIsSmiles {
		body := strings.NewReader(url.Values{"smiles": {identifier}}.Encode())
		resp, err = c.http.Post(path, "application/x-www-form-urlencoded", body)
	} else {
		getURL := fmt.Sprintf("%s/compound/%s/%s/property/%s/JSON",
			c.baseURL, namespace, url.PathEscape(identifier), propertyFields)
		resp, err = c.http.Get(getURL)
	}
	if err != nil {
		return propertyResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return propertyResponse{}, errNotFound
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

// fetchSVG fetches the 2D structure SVG for a CID. Failure is non-fatal.
func (c *pubchemClient) fetchSVG(cid int) (template.HTML, error) {
	u := fmt.Sprintf("%s/compound/cid/%d/record/SVG?record_type=2d", c.baseURL, cid)
	resp, err := c.http.Get(u)
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

// fetchCAS scans PubChem synonyms for the first CAS registry number.
func (c *pubchemClient) fetchCAS(cid int) (string, error) {
	u := fmt.Sprintf("%s/compound/cid/%d/synonyms/JSON", c.baseURL, cid)
	resp, err := c.http.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("synonyms fetch returned %d", resp.StatusCode)
	}
	var sr synonymResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return "", err
	}
	if len(sr.InformationList.Information) == 0 {
		return "", nil
	}
	for _, syn := range sr.InformationList.Information[0].Synonym {
		if casRE.MatchString(syn) {
			return syn, nil
		}
	}
	return "", nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -run TestFetch -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/client.go internal/resolver/client_test.go
git commit -m "feat: add shared PubChem HTTP client"
```

---

### Task 3: SmilesResolver

**Files:**
- Create: `internal/resolver/smiles.go`
- Create: `internal/resolver/smiles_test.go`

**Interfaces:**
- Consumes: `pubchemClient`, `CompoundResult`, `Resolver`, `errNotFound`, `batchWorkers`
- Produces: `SmilesResolver` (unexported struct), `NewSmilesResolver() Resolver`

- [ ] **Step 1: Write the failing test**

```go
// internal/resolver/smiles_test.go
package resolver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newSmilesTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/compound/smiles/property/"):
			json.NewEncoder(w).Encode(propertyResponse{
				PropertyTable: propertyTable{Properties: []propertyRow{{
					CID: 180, IUPACName: "propan-2-one",
					MolecularFormula: "C3H6O", MolecularWeight: "58.08",
					InChIKey: "CSCPPACGZOOCGX-UHFFFAOYSA-N",
					CanonicalSMILES: "CC(C)=O", IsomericSMILES: "CC(C)=O",
				}}},
			})
		case strings.Contains(r.URL.Path, fmt.Sprintf("/compound/cid/%d/record/SVG", 180)):
			w.Header().Set("Content-Type", "image/svg+xml")
			fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg"><text>mock</text></svg>`)
		case strings.Contains(r.URL.Path, fmt.Sprintf("/compound/cid/%d/synonyms/JSON", 180)):
			json.NewEncoder(w).Encode(synonymResponse{
				InformationList: synonymInfo{
					Information: []synonymEntry{{CID: 180, Synonym: []string{"acetone", "67-64-1"}}},
				},
			})
		default:
			http.Error(w, "unexpected: "+r.URL.Path, 404)
		}
	}))
}

func TestSmilesResolver_Resolve(t *testing.T) {
	srv := newSmilesTestServer(t)
	defer srv.Close()

	r := &SmilesResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	got, err := r.Resolve("CC(C)=O")
	if err != nil {
		t.Fatal(err)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
	if got.CAS != "67-64-1" {
		t.Errorf("CAS: got %q, want %q", got.CAS, "67-64-1")
	}
	if got.CID != 180 {
		t.Errorf("CID: got %d, want 180", got.CID)
	}
	if got.SVG == "" {
		t.Error("expected non-empty SVG")
	}
}

func TestSmilesResolver_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	r := &SmilesResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	got, err := r.Resolve("INVALID")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == "" {
		t.Error("expected non-empty Error for not-found result")
	}
}

func TestSmilesResolver_Batch(t *testing.T) {
	srv := newSmilesTestServer(t)
	defer srv.Close()

	r := &SmilesResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	results, err := r.Batch([]string{"CC(C)=O", "CC(C)=O"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, res := range results {
		if res.IUPAC != "propan-2-one" {
			t.Errorf("IUPAC: got %q, want %q", res.IUPAC, "propan-2-one")
		}
		if res.SVG != "" {
			t.Error("batch results should not include SVG")
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -run TestSmilesResolver -v
```
Expected: compilation error (SmilesResolver not defined).

- [ ] **Step 3: Write `internal/resolver/smiles.go`**

```go
package resolver

import (
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
	return r.resolve(input, true)
}

func (r *SmilesResolver) resolve(input string, fetchSVG bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	props, err := r.client.fetchProperties("smiles", input, true)
	if err == errNotFound {
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
	result.CID       = p.CID
	result.IUPAC     = p.IUPACName
	result.Canonical = p.CanonicalSMILES
	result.Isomeric  = p.IsomericSMILES
	result.Formula   = p.MolecularFormula
	result.MW        = p.MolecularWeight
	result.InChIKey  = p.InChIKey

	if cas, _ := r.client.fetchCAS(p.CID); cas != "" {
		result.CAS = cas
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
}

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
			res, err := r.resolve(in, false)
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

- [ ] **Step 4: Run tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -run TestSmilesResolver -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/smiles.go internal/resolver/smiles_test.go
git commit -m "feat: add SmilesResolver"
```

---

### Task 4: NameResolver

**Files:**
- Create: `internal/resolver/name.go`
- Create: `internal/resolver/name_test.go`

**Interfaces:**
- Consumes: `pubchemClient`, `CompoundResult`, `Resolver`, `errNotFound`, `batchWorkers` (all from same package)
- Produces: `NameResolver` (unexported struct), `NewNameResolver() Resolver`

- [ ] **Step 1: Write the failing test**

```go
// internal/resolver/name_test.go
package resolver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newNameTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/compound/name/") && strings.Contains(r.URL.Path, "/property/"):
			json.NewEncoder(w).Encode(propertyResponse{
				PropertyTable: propertyTable{Properties: []propertyRow{{
					CID: 180, IUPACName: "propan-2-one",
					MolecularFormula: "C3H6O", MolecularWeight: "58.08",
					InChIKey: "CSCPPACGZOOCGX-UHFFFAOYSA-N",
					CanonicalSMILES: "CC(C)=O", IsomericSMILES: "CC(C)=O",
				}}},
			})
		case strings.Contains(r.URL.Path, fmt.Sprintf("/compound/cid/%d/record/SVG", 180)):
			w.Header().Set("Content-Type", "image/svg+xml")
			fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg"><text>mock</text></svg>`)
		case strings.Contains(r.URL.Path, fmt.Sprintf("/compound/cid/%d/synonyms/JSON", 180)):
			json.NewEncoder(w).Encode(synonymResponse{
				InformationList: synonymInfo{
					Information: []synonymEntry{{CID: 180, Synonym: []string{"acetone", "67-64-1"}}},
				},
			})
		default:
			http.Error(w, "unexpected: "+r.URL.Path, 404)
		}
	}))
}

func TestNameResolver_Resolve(t *testing.T) {
	srv := newNameTestServer(t)
	defer srv.Close()

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	got, err := r.Resolve("acetone")
	if err != nil {
		t.Fatal(err)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
	if got.Canonical != "CC(C)=O" {
		t.Errorf("SMILES: got %q, want %q", got.Canonical, "CC(C)=O")
	}
	if got.CAS != "67-64-1" {
		t.Errorf("CAS: got %q, want %q", got.CAS, "67-64-1")
	}
	if got.SVG == "" {
		t.Error("expected non-empty SVG")
	}
}

func TestNameResolver_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	got, err := r.Resolve("notacompound12345xyz")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == "" {
		t.Error("expected non-empty Error for not-found result")
	}
}

func TestNameResolver_Batch(t *testing.T) {
	srv := newNameTestServer(t)
	defer srv.Close()

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, http: srv.Client()}}
	results, err := r.Batch([]string{"acetone", "acetone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for _, res := range results {
		if res.Canonical != "CC(C)=O" {
			t.Errorf("SMILES: got %q, want %q", res.Canonical, "CC(C)=O")
		}
		if res.SVG != "" {
			t.Error("batch results should not include SVG")
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -run TestNameResolver -v
```
Expected: compilation error (NameResolver not defined).

- [ ] **Step 3: Write `internal/resolver/name.go`**

```go
package resolver

import (
	"sync"
	"time"
)

type NameResolver struct {
	client *pubchemClient
}

func NewNameResolver() Resolver {
	return &NameResolver{client: newPubchemClient()}
}

func (r *NameResolver) SystemID() string { return "name" }
func (r *NameResolver) Name() string     { return "Name / CAS" }

func (r *NameResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(input, true)
}

func (r *NameResolver) resolve(input string, fetchSVG bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	props, err := r.client.fetchProperties("name", input, false)
	if err == errNotFound {
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
	result.CID       = p.CID
	result.IUPAC     = p.IUPACName
	result.Canonical = p.CanonicalSMILES
	result.Isomeric  = p.IsomericSMILES
	result.Formula   = p.MolecularFormula
	result.MW        = p.MolecularWeight
	result.InChIKey  = p.InChIKey

	if cas, _ := r.client.fetchCAS(p.CID); cas != "" {
		result.CAS = cas
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
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
			res, err := r.resolve(in, false)
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

- [ ] **Step 4: Run all resolver tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -v
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/resolver/name.go internal/resolver/name_test.go
git commit -m "feat: add NameResolver"
```

---

### Task 5: Handlers

**Files:**
- Create: `internal/handlers/templates.go`
- Create: `internal/handlers/resolve.go`
- Create: `internal/handlers/batch.go`
- Create: `internal/handlers/export.go`
- Create: `internal/handlers/resolve_test.go`
- Create: `internal/handlers/batch_test.go`

**Interfaces:**
- Consumes: `resolver.Resolver`, `resolver.CompoundResult`
- Produces:
  - `MustLoadTemplates(dir string) *template.Template`
  - `NewResolveHandler(tmpl *template.Template, r resolver.Resolver) http.Handler`
  - `NewBatchHandler(tmpl *template.Template, r resolver.Resolver) http.Handler`
  - `NewExportHandler(tmpl *template.Template) http.Handler`

- [ ] **Step 1: Write the failing tests**

```go
// internal/handlers/resolve_test.go
package handlers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/roncofaber/chem-resolver/internal/resolver"
)

type stubResolver struct {
	result resolver.CompoundResult
	err    error
}

func (s *stubResolver) SystemID() string { return "smiles" }
func (s *stubResolver) Name() string     { return "SMILES" }
func (s *stubResolver) Resolve(input string) (resolver.CompoundResult, error) {
	s.result.Input = input
	return s.result, s.err
}
func (s *stubResolver) Batch(inputs []string) ([]resolver.CompoundResult, error) {
	results := make([]resolver.CompoundResult, len(inputs))
	for i, in := range inputs {
		res := s.result
		res.Input = in
		results[i] = res
	}
	return results, s.err
}

func mustParseResultTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("result.html").Parse(
		`{{define "result.html"}}{{if .Error}}ERROR:{{.Error}}{{else}}OK:{{.IUPAC}}{{end}}{{end}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func TestResolveHandler_Post(t *testing.T) {
	stub := &stubResolver{result: resolver.CompoundResult{
		IUPAC: "propan-2-one", CID: 180, ResolvedAt: time.Now(),
	}}
	h := NewResolveHandler(mustParseResultTemplate(t), stub)

	form := url.Values{"input": {"CC(C)=O"}}
	req := httptest.NewRequest(http.MethodPost, "/smiles/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "propan-2-one") {
		t.Errorf("body missing IUPAC name: %q", w.Body.String())
	}
}

func TestResolveHandler_EmptyInput(t *testing.T) {
	stub := &stubResolver{}
	h := NewResolveHandler(mustParseResultTemplate(t), stub)

	form := url.Values{"input": {""}}
	req := httptest.NewRequest(http.MethodPost, "/smiles/resolve", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "ERROR") {
		t.Errorf("expected error for empty input, got: %q", w.Body.String())
	}
}

func TestResolveHandler_MethodNotAllowed(t *testing.T) {
	stub := &stubResolver{}
	h := NewResolveHandler(mustParseResultTemplate(t), stub)

	req := httptest.NewRequest(http.MethodGet, "/smiles/resolve", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
```

```go
// internal/handlers/batch_test.go
package handlers

import (
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roncofaber/chem-resolver/internal/resolver"
)

func mustParseBatchTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("batch_result.html").Parse(
		`{{define "batch_result.html"}}{{if .Error}}ERROR{{else}}TOTAL:{{.Summary.Total}}{{end}}{{end}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func TestBatchHandler_TextareaInput(t *testing.T) {
	stub := &stubResolver{result: resolver.CompoundResult{
		IUPAC: "propan-2-one", ResolvedAt: time.Now(),
	}}
	h := NewBatchHandler(mustParseBatchTemplate(t), stub)

	var b strings.Builder
	mw := multipart.NewWriter(&b)
	mw.WriteField("inputs", "CC(C)=O\nC")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/smiles/batch", strings.NewReader(b.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	if !strings.Contains(w.Body.String(), "TOTAL:2") {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestBatchHandler_EmptyInput(t *testing.T) {
	stub := &stubResolver{}
	h := NewBatchHandler(mustParseBatchTemplate(t), stub)

	var b strings.Builder
	mw := multipart.NewWriter(&b)
	mw.WriteField("inputs", "")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/smiles/batch", strings.NewReader(b.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "ERROR") {
		t.Errorf("expected error for empty input, got: %q", w.Body.String())
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/handlers/ -v
```
Expected: compilation error (handlers not defined).

- [ ] **Step 3: Write `internal/handlers/templates.go`**

```go
package handlers

import (
	"html/template"
	"path/filepath"
)

func MustLoadTemplates(dir string) *template.Template {
	tmpl, err := template.ParseGlob(filepath.Join(dir, "*.html"))
	if err != nil {
		panic("failed to parse base templates: " + err.Error())
	}
	partials, err := filepath.Glob(filepath.Join(dir, "partials", "*.html"))
	if err != nil {
		panic("failed to glob partials: " + err.Error())
	}
	if len(partials) > 0 {
		if tmpl, err = tmpl.ParseFiles(partials...); err != nil {
			panic("failed to parse partial templates: " + err.Error())
		}
	}
	tabs, err := filepath.Glob(filepath.Join(dir, "*/tab.html"))
	if err != nil {
		panic("failed to glob tab templates: " + err.Error())
	}
	if len(tabs) > 0 {
		if tmpl, err = tmpl.ParseFiles(tabs...); err != nil {
			panic("failed to parse tab templates: " + err.Error())
		}
	}
	return tmpl
}
```

- [ ] **Step 4: Write `internal/handlers/resolve.go`**

```go
package handlers

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chem-resolver/internal/resolver"
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
	res, err := h.resolver.Resolve(input)
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

- [ ] **Step 5: Write `internal/handlers/batch.go`**

```go
package handlers

import (
	"bufio"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chem-resolver/internal/resolver"
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

	results, _ := h.resolver.Batch(inputs)

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

- [ ] **Step 6: Write `internal/handlers/export.go`**

```go
package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/roncofaber/chem-resolver/internal/resolver"
)

type ExportHandler struct {
	tmpl *template.Template
}

func NewExportHandler(tmpl *template.Template) *ExportHandler {
	return &ExportHandler{tmpl: tmpl}
}

func (h *ExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var results []resolver.CompoundResult
	if err := json.Unmarshal([]byte(r.FormValue("results")), &results); err != nil {
		http.Error(w, "invalid results data", http.StatusBadRequest)
		return
	}
	system := r.FormValue("system")
	if system == "" {
		system = "chem"
	}
	filename := fmt.Sprintf("%s_%s.csv", system, time.Now().UTC().Format("20060102_150405"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	cw := csv.NewWriter(w)
	cw.Write([]string{"Input", "CID", "IUPAC", "Formula", "MW", "CAS", "InChIKey", "CanonicalSMILES", "IsomericSMILES", "Error"})
	for _, res := range results {
		cid := ""
		if res.CID != 0 {
			cid = fmt.Sprintf("%d", res.CID)
		}
		cw.Write([]string{
			res.Input, cid, res.IUPAC, res.Formula, res.MW,
			res.CAS, res.InChIKey, res.Canonical, res.Isomeric, res.Error,
		})
	}
	cw.Flush()
}
```

- [ ] **Step 7: Run all handler tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/handlers/ -v
```
Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/handlers/
git commit -m "feat: add resolve, batch, and export handlers"
```

---

### Task 6: Templates + CSS

**Files:**
- Create: `templates/icons.html`
- Create: `templates/index.html`
- Create: `templates/smiles/tab.html`
- Create: `templates/name/tab.html`
- Create: `templates/partials/result.html`
- Create: `templates/partials/batch_result.html`
- Create: `static/style.css`

No unit tests — visual output verified by the smoke test in Task 7.

- [ ] **Step 1: Write `templates/icons.html`**

```html
{{define "icon/rotate-ccw"}}
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>
  <path d="M3 3v5h5"/>
</svg>
{{end}}

{{define "icon/copy"}}
<svg class="icon-copy" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
  <rect x="5" y="4" width="8" height="10" rx="1.5"/>
  <path d="M3 12H2.5A1.5 1.5 0 0 1 1 10.5v-8A1.5 1.5 0 0 1 2.5 1h7A1.5 1.5 0 0 1 11 2.5V4"/>
</svg>
{{end}}

{{define "icon/check"}}
<svg class="icon-check" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <polyline points="2.5,8.5 6,12 13.5,4.5"/>
</svg>
{{end}}
```

- [ ] **Step 2: Write `templates/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Chem Resolver</title>
  <link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>⚗</text></svg>">
  <link rel="stylesheet" href="/static/style.css">
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
  <div class="page-wrap">
    <header class="site-header">
      <h1><span class="h1-primary">Chem</span><span class="h1-validator">&nbsp;&nbsp;Resolver</span></h1>
      <p>Resolve chemical identifiers via the PubChem REST API</p>
    </header>

    <div class="tab-bar">
      {{range $i, $r := .Resolvers}}
      <button class="tab-btn{{if eq $i 0}} active{{end}}"
              data-tab="{{$r.SystemID}}"
              onclick="switchTab('{{$r.SystemID}}')">
        {{$r.Name}}
      </button>
      {{end}}
    </div>

    {{range $i, $r := .Resolvers}}
    <div id="tab-{{$r.SystemID}}" class="tab-panel{{if eq $i 0}} active{{end}}">
      {{if eq $r.SystemID "smiles"}}{{template "smiles/tab.html" $r}}{{end}}
      {{if eq $r.SystemID "name"}}{{template "name/tab.html" $r}}{{end}}
    </div>
    {{end}}
  </div>

  <footer class="site-footer">
    <span><a href="https://pubchem.ncbi.nlm.nih.gov/" target="_blank" rel="noopener">PubChem REST API</a></span>
    <a href="https://github.com/roncofaber/chem-resolver" target="_blank" rel="noopener">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" style="width:13px;height:13px;vertical-align:middle;margin-right:0.2rem;">
        <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844a9.59 9.59 0 0 1 2.504.337c1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.02 10.02 0 0 0 22 12.017C22 6.484 17.522 2 12 2z"/>
      </svg>GitHub</a>
  </footer>

  <script>
  function switchTab(id) {
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    document.getElementById('tab-' + id).classList.add('active');
    document.querySelector('[data-tab="' + id + '"]').classList.add('active');
  }
  </script>
</body>
</html>
```

- [ ] **Step 3: Write `templates/smiles/tab.html`**

```html
{{define "smiles/tab.html"}}
<section class="card">
  <p class="card-title">Single Lookup</p>
  <button class="card-reset" id="smiles-single-reset" aria-label="Clear"
          onclick="document.getElementById('smiles-single-form').reset();
                   document.getElementById('smiles-result').innerHTML='';
                   this.style.display='none';"
          style="display:none">
    {{template "icon/rotate-ccw" .}}
    Clear
  </button>
  <form id="smiles-single-form"
        hx-post="/smiles/resolve"
        hx-target="#smiles-result"
        hx-swap="innerHTML"
        hx-indicator="#smiles-single-indicator"
        hx-on:submit="document.getElementById('smiles-single-reset').style.display='flex';">
    <label for="smiles-input">SMILES</label>
    <input type="text" id="smiles-input" name="input"
           placeholder="e.g. CC(=O)C or c1ccccc1"
           autocomplete="off" spellcheck="false">
    <div class="mt-sm">
      <button class="btn" type="submit">Resolve</button>
      <span id="smiles-single-indicator" class="indicator">Resolving</span>
    </div>
  </form>
  <div id="smiles-result"></div>
</section>

<section class="card">
  <p class="card-title">Batch</p>
  <button class="card-reset" id="smiles-batch-reset" aria-label="Clear"
          onclick="document.getElementById('smiles-batch-form').reset();
                   document.getElementById('smiles-batch-result').innerHTML='';
                   this.style.display='none';"
          style="display:none">
    {{template "icon/rotate-ccw" .}}
    Clear
  </button>
  <form id="smiles-batch-form"
        hx-post="/smiles/batch"
        hx-target="#smiles-batch-result"
        hx-swap="innerHTML"
        hx-indicator="#smiles-batch-indicator, #smiles-batch-progress"
        hx-encoding="multipart/form-data"
        hx-on:submit="document.getElementById('smiles-batch-reset').style.display='flex';
                      document.getElementById('smiles-batch-result').innerHTML='';">
    <label for="smiles-inputs">SMILES (one per line)</label>
    <textarea id="smiles-inputs" name="inputs" rows="5"
              placeholder="CC(=O)C&#10;c1ccccc1&#10;CCO"
              spellcheck="false"></textarea>
    <p class="hint">Or upload a plain text file:</p>
    <input type="file" name="file" accept=".txt,.csv">
    <div>
      <button class="btn" type="submit">Resolve All</button>
      <span id="smiles-batch-indicator" class="indicator">Processing</span>
    </div>
  </form>
  <div class="progress-bar" id="smiles-batch-progress"></div>
  <div id="smiles-batch-result"></div>
</section>
{{end}}
```

- [ ] **Step 4: Write `templates/name/tab.html`**

```html
{{define "name/tab.html"}}
<section class="card">
  <p class="card-title">Single Lookup</p>
  <button class="card-reset" id="name-single-reset" aria-label="Clear"
          onclick="document.getElementById('name-single-form').reset();
                   document.getElementById('name-result').innerHTML='';
                   this.style.display='none';"
          style="display:none">
    {{template "icon/rotate-ccw" .}}
    Clear
  </button>
  <form id="name-single-form"
        hx-post="/name/resolve"
        hx-target="#name-result"
        hx-swap="innerHTML"
        hx-indicator="#name-single-indicator"
        hx-on:submit="document.getElementById('name-single-reset').style.display='flex';">
    <label for="name-input">Name or CAS</label>
    <input type="text" id="name-input" name="input"
           placeholder="e.g. acetone or 67-64-1 or propan-2-one"
           autocomplete="off" spellcheck="false">
    <div class="mt-sm">
      <button class="btn" type="submit">Resolve</button>
      <span id="name-single-indicator" class="indicator">Resolving</span>
    </div>
  </form>
  <div id="name-result"></div>
</section>

<section class="card">
  <p class="card-title">Batch</p>
  <button class="card-reset" id="name-batch-reset" aria-label="Clear"
          onclick="document.getElementById('name-batch-form').reset();
                   document.getElementById('name-batch-result').innerHTML='';
                   this.style.display='none';"
          style="display:none">
    {{template "icon/rotate-ccw" .}}
    Clear
  </button>
  <form id="name-batch-form"
        hx-post="/name/batch"
        hx-target="#name-batch-result"
        hx-swap="innerHTML"
        hx-indicator="#name-batch-indicator, #name-batch-progress"
        hx-encoding="multipart/form-data"
        hx-on:submit="document.getElementById('name-batch-reset').style.display='flex';
                      document.getElementById('name-batch-result').innerHTML='';">
    <label for="name-inputs">Names or CAS numbers (one per line)</label>
    <textarea id="name-inputs" name="inputs" rows="5"
              placeholder="acetone&#10;67-64-1&#10;benzene"
              spellcheck="false"></textarea>
    <p class="hint">Or upload a plain text file:</p>
    <input type="file" name="file" accept=".txt,.csv">
    <div>
      <button class="btn" type="submit">Resolve All</button>
      <span id="name-batch-indicator" class="indicator">Processing</span>
    </div>
  </form>
  <div class="progress-bar" id="name-batch-progress"></div>
  <div id="name-batch-result"></div>
</section>
{{end}}
```

- [ ] **Step 5: Write `templates/partials/result.html`**

```html
{{if .Error}}
<div class="result-box error">
  <span class="status-badge error-tag">Error</span>
  <p>{{.Error}}</p>
</div>
{{else}}
<div class="result-box valid">
  <span class="status-badge valid">Found</span>
  {{if .SVG}}
  <div class="structure-wrap">{{.SVG}}</div>
  {{end}}
  <table class="result-fields">
    <tr><td>IUPAC</td><td>{{.IUPAC}}</td></tr>
    <tr><td>Formula</td><td class="code-value">{{.Formula}}</td></tr>
    <tr><td>MW</td><td>{{.MW}}</td></tr>
    {{if .CAS}}<tr><td>CAS</td><td class="code-value">{{.CAS}}</td></tr>{{end}}
    <tr><td>InChIKey</td><td class="code-value">{{.InChIKey}}</td></tr>
    <tr class="copyable-row">
      <td>SMILES</td>
      <td>
        <span class="field-value code-value">{{.Canonical}}</span>
        <button class="copy-btn" onclick="copyText(this, this.previousElementSibling.textContent)" aria-label="Copy SMILES">
          {{template "icon/copy" .}}
          {{template "icon/check" .}}
        </button>
      </td>
    </tr>
    {{if ne .Isomeric .Canonical}}
    <tr class="copyable-row">
      <td>Isomeric</td>
      <td>
        <span class="field-value code-value">{{.Isomeric}}</span>
        <button class="copy-btn" onclick="copyText(this, this.previousElementSibling.textContent)" aria-label="Copy isomeric SMILES">
          {{template "icon/copy" .}}
          {{template "icon/check" .}}
        </button>
      </td>
    </tr>
    {{end}}
    <tr><td>CID</td><td><a href="https://pubchem.ncbi.nlm.nih.gov/compound/{{.CID}}" target="_blank" rel="noopener">{{.CID}}</a></td></tr>
    <tr><td>Resolved</td><td>{{.ResolvedAt.Format "2006-01-02 15:04:05 UTC"}}</td></tr>
  </table>
</div>

<script>
htmx.process(document.currentScript.parentElement);
function copyText(btn, text) {
  navigator.clipboard.writeText(text.trim()).then(function() {
    btn.classList.add('copy-btn--done');
    setTimeout(function() { btn.classList.remove('copy-btn--done'); }, 1500);
  });
}
</script>
{{end}}
```

- [ ] **Step 6: Write `templates/partials/batch_result.html`**

```html
{{if .Error}}
<div class="result-box error mt-md">
  <span class="status-badge error-tag">Error</span>
  <p>{{.Error}}</p>
</div>
{{else}}
<div class="mt-md">
  <div class="summary-bar">
    <span class="summary-pill valid">{{.Summary.Found}} found</span>
    <span class="summary-pill invalid">{{.Summary.NotFound}} not found</span>
    {{if .Summary.Errors}}<span class="summary-pill warn">{{.Summary.Errors}} errors</span>{{end}}
    <span class="summary-pill neutral">{{.Summary.Total}} total</span>
    <form method="post" action="/export" class="ml-auto">
      <input type="hidden" name="results" value="{{.ResultsJSON}}">
      <input type="hidden" name="system" value="{{.SystemID}}">
      <button class="btn btn-sm" type="submit">↓ Export CSV</button>
    </form>
  </div>
  <div class="batch-table-wrap">
    <table class="batch-table">
      <thead>
        <tr>
          <th>Input</th>
          <th>Status</th>
          <th>IUPAC</th>
          <th>Formula</th>
          <th>CAS</th>
        </tr>
      </thead>
      <tbody>
        {{range .Results}}
        <tr>
          <td class="col-code">{{.Input}}</td>
          <td>
            {{if .Error}}<span class="chip error-tag">Error</span>
            {{else}}<span class="chip valid">Found</span>{{end}}
          </td>
          <td class="col-name">{{.IUPAC}}</td>
          <td class="col-code">{{.Formula}}</td>
          <td class="col-code">{{.CAS}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </div>
</div>
{{end}}
```

- [ ] **Step 7: Write `static/style.css`**

Copy `static/style.css` verbatim from `/home/roncofaber/software/loinc-validator/static/style.css`, then append these rules:

```css
/* ── Structure image ─────────────────────────────────────────── */
.structure-wrap {
  float: right;
  margin: 0 0 0.75rem 1.25rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  padding: 0.25rem;
  line-height: 0;
}

.structure-wrap svg {
  width: 200px;
  height: 150px;
  display: block;
}

/* ── Textarea (batch input) ──────────────────────────────────── */
textarea {
  width: 100%;
  padding: 0.65rem 0.9rem;
  border: 1px solid var(--border-dark);
  border-radius: var(--radius);
  font-family: var(--font-mono);
  font-size: 0.875rem;
  color: var(--text);
  background: var(--bg);
  outline: none;
  resize: vertical;
  transition: border-color 0.15s, box-shadow 0.15s;
  margin-bottom: 0.9rem;
}

textarea:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(26,92,58,0.12);
}
```

- [ ] **Step 8: Commit**

```bash
git add templates/ static/
git commit -m "feat: add templates and CSS"
```

---

### Task 7: main.go + integration tests + smoke test

**Files:**
- Create: `main.go`
- Create: `internal/resolver/smiles_integration_test.go`
- Create: `internal/resolver/name_integration_test.go`

**Interfaces:**
- Consumes: all previous tasks

- [ ] **Step 1: Write `main.go`**

```go
package main

import (
	"log"
	"net/http"

	"github.com/roncofaber/chem-resolver/internal/handlers"
	"github.com/roncofaber/chem-resolver/internal/resolver"
)

func main() {
	tmpl := handlers.MustLoadTemplates("templates")

	resolvers := []resolver.Resolver{
		resolver.NewSmilesResolver(),
		resolver.NewNameResolver(),
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export", handlers.NewExportHandler(tmpl))

	for _, r := range resolvers {
		id := r.SystemID()
		mux.Handle("/"+id+"/resolve", handlers.NewResolveHandler(tmpl, r))
		mux.Handle("/"+id+"/batch", handlers.NewBatchHandler(tmpl, r))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		tmpl.ExecuteTemplate(w, "index.html", map[string]any{
			"Resolvers": resolvers,
		})
	})

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Build**

```bash
cd /home/roncofaber/software/chem-resolver
go build ./...
```
Expected: no errors.

- [ ] **Step 3: Write integration tests**

```go
// internal/resolver/smiles_integration_test.go
//go:build integration

package resolver

import "testing"

func TestSmilesResolver_Integration_Acetone(t *testing.T) {
	r := NewSmilesResolver()
	got, err := r.Resolve("CC(C)=O")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
	if got.CAS != "67-64-1" {
		t.Errorf("CAS: got %q, want %q", got.CAS, "67-64-1")
	}
	if got.SVG == "" {
		t.Error("expected non-empty SVG")
	}
}

func TestSmilesResolver_Integration_NotFound(t *testing.T) {
	r := NewSmilesResolver()
	got, err := r.Resolve("XXXXXXXXXX")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == "" {
		t.Error("expected not-found error")
	}
}
```

```go
// internal/resolver/name_integration_test.go
//go:build integration

package resolver

import "testing"

func TestNameResolver_Integration_CommonName(t *testing.T) {
	r := NewNameResolver()
	got, err := r.Resolve("acetone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.Canonical != "CC(C)=O" {
		t.Errorf("SMILES: got %q, want %q", got.Canonical, "CC(C)=O")
	}
	if got.SVG == "" {
		t.Error("expected non-empty SVG")
	}
}

func TestNameResolver_Integration_CAS(t *testing.T) {
	r := NewNameResolver()
	got, err := r.Resolve("67-64-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
}
```

- [ ] **Step 4: Run all unit tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 5: Smoke test against the live server**

```bash
cd /home/roncofaber/software/chem-resolver
go run . &
SERVER_PID=$!
sleep 1
curl -s -X POST http://localhost:8080/name/resolve -d "input=acetone" | grep -o 'propan-2-one'
curl -s -X POST http://localhost:8080/smiles/resolve -d "input=CC(C)%3DO" | grep -o 'propan-2-one'
kill $SERVER_PID
```
Expected: `propan-2-one` printed twice.

- [ ] **Step 6: Run integration tests**

```bash
cd /home/roncofaber/software/chem-resolver
go test ./internal/resolver/ -tags integration -v -timeout 60s
```
Expected: all 4 integration tests PASS.

- [ ] **Step 7: Final commit**

```bash
git add main.go internal/resolver/smiles_integration_test.go internal/resolver/name_integration_test.go
git commit -m "feat: wire up main server and add integration tests"
```
