# Batch Progress SSE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current blocking batch POST with a two-step SSE flow — `/batch/start` creates a job and returns immediately, `/batch/stream` streams real progress events as compounds resolve, and the browser fills a progress bar with actual counts.

**Architecture:** A new `internal/jobs` package holds an in-memory `JobStore` keyed by random hex IDs. `AutoResolver.BatchWithProgress` accepts a callback fired after each compound resolves; the start handler passes `job.Incr` as that callback. The SSE stream handler polls the job every 200ms and flushes `event: progress` and `event: done` events. The job uses its own context (independent of any HTTP request) so it survives client disconnects.

**Tech Stack:** Go 1.22, `crypto/rand` (job IDs), `sync/atomic` (done counter), native browser `EventSource` API (no htmx SSE extension).

## Global Constraints

- Module: `github.com/roncofaber/chemres`, Go 1.22
- Job TTL: 10 minutes, swept every minute
- Job IDs: 16 random bytes, hex-encoded (`crypto/rand`)
- SSE poll interval: 200ms
- SSE event names: `progress` (data: `"N/total"`), `done` (data: rendered `batch_result.html`), `error` (data: human-readable message)
- Job context is independent of HTTP request context — client disconnect does NOT cancel the batch
- All existing behaviour preserved: retry logic, X-Forwarded-For, rate limiting, file upload
- `BatchWithProgress` on `Resolver` interface; `Batch` delegates to it with a no-op callback

---

## File Map

| File | Change |
|------|--------|
| `internal/jobs/store.go` | **New** — `Job`, `Snapshot`, `Store` |
| `internal/jobs/store_test.go` | **New** — job lifecycle and TTL tests |
| `internal/resolver/resolver.go` | Add `BatchWithProgress` to interface |
| `internal/resolver/auto.go` | Add `BatchWithProgress`; `Batch` delegates to it |
| `internal/resolver/name.go` | Add stub `BatchWithProgress` |
| `internal/resolver/smiles.go` | Add stub `BatchWithProgress` |
| `internal/handlers/resolve_test.go` | Add `BatchWithProgress` to `stubResolver` |
| `internal/handlers/batch.go` | Rewrite as `BatchStartHandler` — returns JSON job ID |
| `internal/handlers/batch_test.go` | Rewrite tests for new JSON-returning handler |
| `internal/handlers/batchstream.go` | **New** — SSE stream handler |
| `internal/handlers/batchstream_test.go` | **New** — SSE handler tests |
| `main.go` | Register `/batch/start`, `/batch/stream`; inject `*jobs.Store` |
| `templates/index.html` | Replace htmx batch form with JS EventSource flow |
| `static/style.css` | Add `.progress-fill`, `.progress-bar.active` |

---

## Task 1: Job Store

**Files:**
- Create: `internal/jobs/store.go`
- Create: `internal/jobs/store_test.go`

**Interfaces:**
- Produces:
  - `jobs.Store` — `New(total int) (id string, job *Job)`, `Get(id string) (*Job, bool)`
  - `jobs.Job` — `Incr()`, `Finish(results []resolver.CompoundResult, err error)`, `Snapshot() Snapshot`, field `Ctx context.Context`
  - `jobs.Snapshot` — `Done, Total int`, `Finished bool`, `Results []resolver.CompoundResult`, `Err error`
  - `jobs.NewStore() *Store` — starts sweep goroutine

- [ ] **Step 1: Write failing tests**

Create `internal/jobs/store_test.go`:

```go
package jobs

import (
	"testing"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

func newTestStore() *Store { return &Store{} }

func TestStore_New(t *testing.T) {
	s := newTestStore()
	id, job := s.New(10)
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	got, ok := s.Get(id)
	if !ok || got != job {
		t.Fatal("job not retrievable after creation")
	}
}

func TestStore_Get_Missing(t *testing.T) {
	s := newTestStore()
	_, ok := s.Get("does-not-exist")
	if ok {
		t.Fatal("expected false for unknown ID")
	}
}

func TestJob_Incr(t *testing.T) {
	s := newTestStore()
	_, job := s.New(5)
	job.Incr()
	job.Incr()
	job.Incr()
	snap := job.Snapshot()
	if snap.Done != 3 {
		t.Errorf("done: got %d, want 3", snap.Done)
	}
	if snap.Total != 5 {
		t.Errorf("total: got %d, want 5", snap.Total)
	}
}

func TestJob_Finish_Success(t *testing.T) {
	s := newTestStore()
	_, job := s.New(2)
	results := []resolver.CompoundResult{{IUPAC: "water"}}
	job.Finish(results, nil)
	snap := job.Snapshot()
	if !snap.Finished {
		t.Fatal("expected finished=true")
	}
	if len(snap.Results) != 1 || snap.Results[0].IUPAC != "water" {
		t.Errorf("unexpected results: %v", snap.Results)
	}
	if snap.Err != nil {
		t.Errorf("unexpected err: %v", snap.Err)
	}
}

func TestJob_Finish_Error(t *testing.T) {
	s := newTestStore()
	_, job := s.New(1)
	job.Finish(nil, errSentinel)
	snap := job.Snapshot()
	if !snap.Finished {
		t.Fatal("expected finished=true")
	}
	if snap.Err != errSentinel {
		t.Errorf("err: got %v, want sentinel", snap.Err)
	}
}

var errSentinel = fmt.Errorf("sentinel error")

func TestStore_SweepExpired(t *testing.T) {
	s := newTestStore()
	id, job := s.New(1)

	job.mu.Lock()
	job.created = time.Now().Add(-11 * time.Minute)
	job.mu.Unlock()

	s.sweepOnce()

	_, ok := s.Get(id)
	if ok {
		t.Fatal("expected expired job to be removed")
	}
	select {
	case <-job.Ctx.Done():
	default:
		t.Fatal("expected job context to be cancelled after sweep")
	}
}

func TestStore_SweepKeepsFresh(t *testing.T) {
	s := newTestStore()
	id, _ := s.New(1)
	s.sweepOnce()
	_, ok := s.Get(id)
	if !ok {
		t.Fatal("fresh job should not be swept")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/jobs/ -v
```

Expected: compile error — package `jobs` does not exist.

- [ ] **Step 3: Implement the job store**

Create `internal/jobs/store.go`:

```go
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/roncofaber/chemres/internal/resolver"
)

const TTL = 10 * time.Minute

type Snapshot struct {
	Done     int
	Total    int
	Finished bool
	Results  []resolver.CompoundResult
	Err      error
}

type Job struct {
	mu       sync.Mutex
	total    int
	done     int
	finished bool
	results  []resolver.CompoundResult
	err      error
	created  time.Time
	Ctx      context.Context
	cancel   context.CancelFunc
}

func (j *Job) Incr() {
	j.mu.Lock()
	j.done++
	j.mu.Unlock()
}

func (j *Job) Finish(results []resolver.CompoundResult, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.results = results
	j.err = err
	j.finished = true
}

func (j *Job) Snapshot() Snapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Snapshot{
		Done:     j.done,
		Total:    j.total,
		Finished: j.finished,
		Results:  j.results,
		Err:      j.err,
	}
}

type Store struct {
	jobs sync.Map
}

func NewStore() *Store {
	s := &Store{}
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			s.sweepOnce()
		}
	}()
	return s
}

func (s *Store) New(total int) (string, *Job) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("jobs: rand.Read: %v", err))
	}
	id := hex.EncodeToString(b)
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{
		total:   total,
		created: time.Now(),
		Ctx:     ctx,
		cancel:  cancel,
	}
	s.jobs.Store(id, job)
	return id, job
}

func (s *Store) Get(id string) (*Job, bool) {
	v, ok := s.jobs.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*Job), true
}

func (s *Store) sweepOnce() {
	now := time.Now()
	s.jobs.Range(func(k, v interface{}) bool {
		job := v.(*Job)
		job.mu.Lock()
		expired := now.Sub(job.created) > TTL
		job.mu.Unlock()
		if expired {
			job.cancel()
			s.jobs.Delete(k)
		}
		return true
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/jobs/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/store.go internal/jobs/store_test.go
git commit -m "feat: add in-memory job store for SSE batch progress"
```

---

## Task 2: BatchWithProgress on Resolver

**Files:**
- Modify: `internal/resolver/resolver.go`
- Modify: `internal/resolver/auto.go`
- Modify: `internal/resolver/name.go`
- Modify: `internal/resolver/smiles.go`
- Modify: `internal/handlers/resolve_test.go` (update `stubResolver`)
- Create: `internal/resolver/auto_test.go`

**Interfaces:**
- Consumes: existing `AutoResolver.Batch` two-phase logic from `auto.go`
- Produces: `BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error)` on `Resolver` interface and all three resolver types; `Batch` becomes a no-op wrapper

- [ ] **Step 1: Write failing test**

Create `internal/resolver/auto_test.go`:

```go
package resolver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAutoResolver_BatchWithProgress_CallbackCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/compound/cid/0/synonyms/JSON" ||
			(len(r.URL.Path) > 20 && r.URL.Path[len(r.URL.Path)-13:] == "/synonyms/JSON") {
			json.NewEncoder(w).Encode(synonymResponse{})
			return
		}
		json.NewEncoder(w).Encode(propertyResponse{
			PropertyTable: propertyTable{
				Properties: []propertyRow{{CID: 1, IUPACName: "water"}},
			},
		})
	}))
	defer srv.Close()

	client := &pubchemClient{baseURL: srv.URL, autocompleteBase: srv.URL, http: srv.Client()}
	r := &AutoResolver{
		smiles: &SmilesResolver{client: client},
		name:   &NameResolver{client: client},
	}

	inputs := []string{"water", "ethanol", "methanol"}
	var callCount int32
	_, err := r.BatchWithProgress(context.Background(), inputs, func(done, total int) {
		atomic.AddInt32(&callCount, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if int(callCount) != len(inputs) {
		t.Errorf("callback called %d times, want %d", callCount, len(inputs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/resolver/ -run TestAutoResolver_BatchWithProgress -v
```

Expected: compile error — `BatchWithProgress` undefined.

- [ ] **Step 3: Add `BatchWithProgress` to the Resolver interface**

In `internal/resolver/resolver.go`, add one line to the interface:

```go
type Resolver interface {
	SystemID() string
	Name()     string
	Resolve(ctx context.Context, input string) (CompoundResult, error)
	Batch(ctx context.Context, inputs []string) ([]CompoundResult, error)
	BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error)
	Suggest(ctx context.Context, query string) ([]string, error)
}
```

- [ ] **Step 4: Add `BatchWithProgress` to AutoResolver and refactor `Batch`**

In `internal/resolver/auto.go`, add `"sync/atomic"` to imports.

Replace the existing `Batch` method with:

```go
func (r *AutoResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup
	var doneCount int32

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
			n := int(atomic.AddInt32(&doneCount, 1))
			onResolve(n, len(inputs))
		}(i, input)
	}
	wg.Wait()

	var cids []int
	cidIdx := make(map[int][]int)
	for i, res := range results {
		if res.CID != 0 {
			if _, seen := cidIdx[res.CID]; !seen {
				cids = append(cids, res.CID)
			}
			cidIdx[res.CID] = append(cidIdx[res.CID], i)
		}
	}

	if len(cids) > 0 {
		synMap, err := r.name.client.fetchSynonymsBatch(ctx, cids)
		if err == nil && synMap != nil {
			for cid, idxs := range cidIdx {
				entry, ok := synMap[cid]
				if !ok {
					continue
				}
				for _, i := range idxs {
					results[i].CAS        = entry.CAS
					results[i].Synonyms   = entry.Synonyms
					results[i].CommonName = firstCommonName(entry.Synonyms)
				}
			}
		}
	}

	return results, nil
}

func (r *AutoResolver) Batch(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	return r.BatchWithProgress(ctx, inputs, func(_, _ int) {})
}
```

- [ ] **Step 5: Add stubs to NameResolver and SmilesResolver**

In `internal/resolver/name.go`, add after `Batch`:

```go
func (r *NameResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error) {
	return r.Batch(ctx, inputs)
}
```

In `internal/resolver/smiles.go`, add after `Batch`:

```go
func (r *SmilesResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error) {
	return r.Batch(ctx, inputs)
}
```

- [ ] **Step 6: Add `BatchWithProgress` to `stubResolver` in `internal/handlers/resolve_test.go`**

Add this method to `stubResolver` (after the existing `Batch` method):

```go
func (s *stubResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]resolver.CompoundResult, error) {
	results := make([]resolver.CompoundResult, len(inputs))
	for i, in := range inputs {
		res := s.result
		res.Input = in
		results[i] = res
		onResolve(i+1, len(inputs))
	}
	return results, s.err
}
```

- [ ] **Step 7: Run all tests**

```bash
go test ./internal/resolver/ ./internal/handlers/ -v
```

Expected: all tests PASS, including new `TestAutoResolver_BatchWithProgress_CallbackCount`.

- [ ] **Step 8: Commit**

```bash
git add internal/resolver/resolver.go internal/resolver/auto.go internal/resolver/name.go internal/resolver/smiles.go internal/resolver/auto_test.go internal/handlers/resolve_test.go
git commit -m "feat: add BatchWithProgress to Resolver interface and AutoResolver"
```

---

## Task 3: Batch Start Handler

**Files:**
- Modify: `internal/handlers/batch.go`
- Modify: `internal/handlers/batch_test.go`

**Interfaces:**
- Consumes: `jobs.Store.New`, `jobs.Job.Incr`, `jobs.Job.Finish`, `jobs.Job.Ctx`, `resolver.Resolver.BatchWithProgress`, `resolver.WithClientIP`, `realClientIP`
- Produces: `NewBatchStartHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStartHandler` at `/batch/start` (POST), returns JSON `{"job":"<id>","total":<n>}` or `{"error":"<msg>"}`

Note: `batchSummary` and `batchData` types stay in `batch.go` — `batchstream.go` uses them since both are in the same package.

- [ ] **Step 1: Write failing tests**

Replace the contents of `internal/handlers/batch_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
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

func newMultipartRequest(t *testing.T, field, value string) *http.Request {
	t.Helper()
	var b strings.Builder
	mw := multipart.NewWriter(&b)
	mw.WriteField(field, value)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/batch/start", strings.NewReader(b.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestBatchStartHandler_ReturnsJobID(t *testing.T) {
	stub := &stubResolver{result: resolver.CompoundResult{IUPAC: "propan-2-one", ResolvedAt: time.Now()}}
	store := &jobs.Store{}
	h := NewBatchStartHandler(mustParseBatchTemplate(t), stub, store)

	req := newMultipartRequest(t, "inputs", "CC(C)=O\nCCO")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	var res struct {
		Job   string `json:"job"`
		Total int    `json:"total"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Job == "" {
		t.Error("expected non-empty job ID")
	}
	if res.Total != 2 {
		t.Errorf("total: got %d, want 2", res.Total)
	}
	if res.Error != "" {
		t.Errorf("unexpected error: %s", res.Error)
	}
}

func TestBatchStartHandler_EmptyInput(t *testing.T) {
	stub := &stubResolver{}
	store := &jobs.Store{}
	h := NewBatchStartHandler(mustParseBatchTemplate(t), stub, store)

	req := newMultipartRequest(t, "inputs", "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var res struct {
		Error string `json:"error"`
	}
	json.NewDecoder(w.Body).Decode(&res)
	if res.Error == "" {
		t.Error("expected error for empty input")
	}
}

func TestBatchStartHandler_SkipsComments(t *testing.T) {
	stub := &stubResolver{result: resolver.CompoundResult{IUPAC: "water", ResolvedAt: time.Now()}}
	store := &jobs.Store{}
	h := NewBatchStartHandler(mustParseBatchTemplate(t), stub, store)

	req := newMultipartRequest(t, "inputs", "water\n# comment\nethanol")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var res struct {
		Total int `json:"total"`
	}
	json.NewDecoder(w.Body).Decode(&res)
	if res.Total != 2 {
		t.Errorf("total: got %d, want 2 (comment should be excluded)", res.Total)
	}
}

// Satisfy compiler — stubResolver.BatchWithProgress already defined in resolve_test.go
var _ context.Context = context.Background()
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/handlers/ -run TestBatchStart -v
```

Expected: compile error — `NewBatchStartHandler` undefined.

- [ ] **Step 3: Rewrite `internal/handlers/batch.go`**

```go
package handlers

import (
	"bufio"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

const maxBatchFileSize = 5 << 20

// batchSummary and batchData are used by both BatchStartHandler (indirectly) and BatchStreamHandler.
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

type BatchStartHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
	store    *jobs.Store
}

func NewBatchStartHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStartHandler {
	return &BatchStartHandler{tmpl: tmpl, resolver: r, store: store}
}

type startResponse struct {
	Job   string `json:"job,omitempty"`
	Total int    `json:"total,omitempty"`
	Error string `json:"error,omitempty"`
}

func (h *BatchStartHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(startResponse{Error: msg})
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(startResponse{Error: "No valid inputs found — enter one per line."})
		return
	}

	ip := realClientIP(r)
	id, job := h.store.New(len(inputs))

	go func() {
		ctx := resolver.WithClientIP(job.Ctx, ip)
		results, err := h.resolver.BatchWithProgress(ctx, inputs, func(_, _ int) { job.Incr() })
		job.Finish(results, err)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(startResponse{Job: id, Total: len(inputs)})
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/handlers/ -run TestBatchStart -v
```

Expected: all three `TestBatchStart*` tests PASS.

- [ ] **Step 5: Run full test suite**

```bash
go test ./...
```

Expected: all packages PASS. (The old `TestBatchHandler_*` tests are replaced; build may fail until main.go is updated in Task 5 — if so, skip this step and run it after Task 5.)

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/batch.go internal/handlers/batch_test.go
git commit -m "feat: replace batch POST handler with job-based BatchStartHandler"
```

---

## Task 4: Batch Stream Handler (SSE)

**Files:**
- Create: `internal/handlers/batchstream.go`
- Create: `internal/handlers/batchstream_test.go`

**Interfaces:**
- Consumes: `jobs.Store.Get`, `jobs.Job.Snapshot`, `jobs.Snapshot`, `batchSummary`, `batchData` (from `batch.go` in same package), template `batch_result.html`
- Produces: `NewBatchStreamHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStreamHandler` at `/batch/stream?job=<id>` (GET)

- [ ] **Step 1: Write failing tests**

Create `internal/handlers/batchstream_test.go`:

```go
package handlers

import (
	"bufio"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

// flushRecorder wraps ResponseRecorder to satisfy http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {}

func mustParseStreamTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("batch_result.html").Parse(
		`{{define "batch_result.html"}}RESULT:{{.Summary.Total}}{{end}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func TestBatchStreamHandler_UnknownJob(t *testing.T) {
	store := &jobs.Store{}
	h := NewBatchStreamHandler(mustParseStreamTemplate(t), &stubResolver{}, store)

	req := httptest.NewRequest(http.MethodGet, "/batch/stream?job=bad", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

func TestBatchStreamHandler_FinishedJob_Success(t *testing.T) {
	store := &jobs.Store{}
	stub := &stubResolver{}
	h := NewBatchStreamHandler(mustParseStreamTemplate(t), stub, store)

	id, job := store.New(2)
	results := []resolver.CompoundResult{
		{IUPAC: "water", Input: "water"},
		{IUPAC: "ethanol", Input: "ethanol"},
	}
	job.Incr()
	job.Incr()
	job.Finish(results, nil)

	req := httptest.NewRequest(http.MethodGet, "/batch/stream?job="+id, nil)
	w := &flushRecorder{httptest.NewRecorder()}
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: progress") {
		t.Errorf("expected progress event, got: %q", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected done event, got: %q", body)
	}
	if !strings.Contains(body, "RESULT:2") {
		t.Errorf("expected rendered result, got: %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want text/event-stream", ct)
	}
}

func TestBatchStreamHandler_FinishedJob_Error(t *testing.T) {
	store := &jobs.Store{}
	h := NewBatchStreamHandler(mustParseStreamTemplate(t), &stubResolver{}, store)

	id, job := store.New(1)
	job.Finish(nil, fmt.Errorf("pubchem down"))

	req := httptest.NewRequest(http.MethodGet, "/batch/stream?job="+id, nil)
	w := &flushRecorder{httptest.NewRecorder()}
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event, got: %q", body)
	}
}
```

Add `"fmt"` to the import block in `batchstream_test.go`.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/handlers/ -run TestBatchStream -v
```

Expected: compile error — `NewBatchStreamHandler` undefined.

- [ ] **Step 3: Implement the SSE stream handler**

Create `internal/handlers/batchstream.go`:

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

type BatchStreamHandler struct {
	tmpl     *template.Template
	resolver resolver.Resolver
	store    *jobs.Store
}

func NewBatchStreamHandler(tmpl *template.Template, r resolver.Resolver, store *jobs.Store) *BatchStreamHandler {
	return &BatchStreamHandler{tmpl: tmpl, resolver: r, store: store}
}

func (h *BatchStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("job")
	job, ok := h.store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastDone := -1

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snap := job.Snapshot()

			if snap.Done != lastDone {
				lastDone = snap.Done
				fmt.Fprintf(w, "event: progress\ndata: %d/%d\n\n", snap.Done, snap.Total)
				flusher.Flush()
			}

			if !snap.Finished {
				continue
			}

			if snap.Err != nil {
				fmt.Fprintf(w, "event: error\ndata: Could not reach PubChem — please try again.\n\n")
				flusher.Flush()
				return
			}

			summary := batchSummary{Total: len(snap.Results)}
			for _, res := range snap.Results {
				switch {
				case res.Error == "Not found in PubChem":
					summary.NotFound++
				case res.Error != "":
					summary.Errors++
				default:
					summary.Found++
				}
			}
			jsonBytes, _ := json.Marshal(snap.Results)
			data := batchData{
				Results:     snap.Results,
				ResultsJSON: string(jsonBytes),
				Summary:     summary,
				SystemID:    h.resolver.SystemID(),
			}
			var buf bytes.Buffer
			h.tmpl.ExecuteTemplate(&buf, "batch_result.html", data)
			// SSE data fields cannot contain bare newlines — collapse to spaces.
			html := bytes.ReplaceAll(buf.Bytes(), []byte("\n"), []byte(" "))
			fmt.Fprintf(w, "event: done\ndata: %s\n\n", html)
			flusher.Flush()
			return
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/handlers/ -run TestBatchStream -v
```

Expected: all three `TestBatchStream*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/batchstream.go internal/handlers/batchstream_test.go
git commit -m "feat: add SSE batch stream handler"
```

---

## Task 5: Wire Routes in main.go

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `jobs.NewStore`, `handlers.NewBatchStartHandler`, `handlers.NewBatchStreamHandler`
- No new interfaces produced.

- [ ] **Step 1: Update `main.go`**

Replace the full contents of `main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/roncofaber/chemres/internal/handlers"
	"github.com/roncofaber/chemres/internal/jobs"
	"github.com/roncofaber/chemres/internal/resolver"
)

func main() {
	tmpl  := handlers.MustLoadTemplates("templates")
	r     := resolver.NewAutoResolver()
	store := jobs.NewStore()

	mux := http.NewServeMux()
	mux.Handle("/static/",      http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/export",       handlers.NewExportHandler(tmpl))
	mux.Handle("/resolve",      handlers.NewResolveHandler(tmpl, r))
	mux.Handle("/batch/start",  handlers.NewBatchStartHandler(tmpl, r, store))
	mux.Handle("/batch/stream", handlers.NewBatchStreamHandler(tmpl, r, store))
	mux.Handle("/suggest",      handlers.NewSuggestHandler(tmpl, r))

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		tmpl.ExecuteTemplate(w, "index.html", nil)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Build and run all tests**

```bash
go build ./... && go test ./...
```

Expected: build succeeds, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire /batch/start and /batch/stream routes"
```

---

## Task 6: Frontend — Progress Bar JS and CSS

**Files:**
- Modify: `templates/index.html`
- Modify: `static/style.css`

**Interfaces:**
- Consumes: `/batch/start` (POST, returns JSON), `/batch/stream?job=<id>` (SSE)
- No Go interfaces.

- [ ] **Step 1: Add CSS for real progress fill**

In `static/style.css`, find the `.progress-bar` block and add `.progress-bar.active` and `.progress-fill` immediately after the existing `@keyframes progress-slide` block:

```css
.progress-bar.active {
  display: block;
  background: var(--border);
}

.progress-fill {
  height: 100%;
  width: 0%;
  background: var(--accent);
  box-shadow: 0 0 8px var(--accent);
  transition: width 0.3s ease;
}
```

- [ ] **Step 2: Update the batch form in `templates/index.html`**

Find the batch `<section class="card">` block and replace it entirely with:

```html
    <section class="card">
      <p class="card-title">Batch<span class="card-title-hint-wrap"><span class="card-title-hint">?</span><span class="card-title-tooltip">Rate limited to 4 requests/s per user via PubChem API</span></span></p>
      <button class="card-reset" id="batch-reset" aria-label="Clear"
              onclick="document.getElementById('batch-form').reset();
                       document.getElementById('batch-result').innerHTML='';
                       hideBatchProgress();
                       this.style.display='none';"
              style="display:none">
        {{template "icon/rotate-ccw" .}}
        Clear
      </button>
      <form id="batch-form" onsubmit="startBatch(event)">
        <label for="batch-inputs">Identifiers (one per line)</label>
        <textarea id="batch-inputs" name="inputs" rows="5"
                  placeholder="CCO&#10;acetone&#10;64-17-5&#10;LFQSCWFLJHTTHZ-UHFFFAOYSA-N"
                  spellcheck="false"></textarea>
        <p class="hint">Accepts SMILES, names, CAS numbers, and InChIKeys. Or upload a plain text file:</p>
        <div class="input-row">
          <input type="file" name="file" accept=".txt,.csv" style="flex:1;margin-bottom:0;align-self:center">
          <button class="btn" type="submit">Resolve All</button>
          <span id="batch-indicator" class="indicator" style="display:none">Processing</span>
        </div>
      </form>
      <div class="progress-bar" id="batch-progress">
        <div class="progress-fill" id="batch-progress-fill"></div>
      </div>
      <div id="batch-result"></div>
    </section>
```

- [ ] **Step 3: Add the batch JS functions**

In `templates/index.html`, inside the `<script>` tag, add these functions before the closing `</script>` tag:

```javascript
  function startBatch(e) {
    e.preventDefault();
    var form = document.getElementById('batch-form');
    var data = new FormData(form);

    document.getElementById('batch-result').innerHTML = '';
    document.getElementById('batch-reset').style.display = 'flex';
    showBatchProgress(0, 'Starting…');

    fetch('/batch/start', { method: 'POST', body: data })
      .then(function(r) { return r.json(); })
      .then(function(res) {
        if (res.error) { hideBatchProgress(); showBatchError(res.error); return; }
        var total = res.total;
        var es = new EventSource('/batch/stream?job=' + res.job);

        es.addEventListener('progress', function(ev) {
          var parts = ev.data.split('/');
          var done = parseInt(parts[0], 10);
          var tot  = parseInt(parts[1], 10);
          showBatchProgress(done / tot, done + ' / ' + tot);
        });

        es.addEventListener('done', function(ev) {
          es.close();
          hideBatchProgress();
          document.getElementById('batch-result').innerHTML = ev.data;
          document.querySelectorAll('.structure-wrap[data-smiles]:not([data-sd-rendered])').forEach(function(el) {
            el.setAttribute('data-sd-rendered', '1');
            drawStructure(el);
          });
        });

        es.addEventListener('error', function(ev) {
          es.close();
          hideBatchProgress();
          showBatchError(ev.data || 'Could not reach PubChem — please try again.');
        });

        es.onerror = function() {
          es.close();
          hideBatchProgress();
          showBatchError('Connection lost — please try again.');
        };
      })
      .catch(function() {
        hideBatchProgress();
        showBatchError('Could not reach server.');
      });
  }

  function showBatchProgress(fraction, label) {
    var bar  = document.getElementById('batch-progress');
    var fill = document.getElementById('batch-progress-fill');
    var ind  = document.getElementById('batch-indicator');
    bar.classList.add('active');
    fill.style.width = (fraction * 100) + '%';
    ind.textContent  = label || 'Processing';
    ind.style.display = 'inline-flex';
  }

  function hideBatchProgress() {
    var bar  = document.getElementById('batch-progress');
    var fill = document.getElementById('batch-progress-fill');
    var ind  = document.getElementById('batch-indicator');
    bar.classList.remove('active');
    fill.style.width  = '0%';
    ind.style.display = 'none';
  }

  function showBatchError(msg) {
    document.getElementById('batch-result').innerHTML =
      '<div class="result-box error mt-md"><span class="status-badge error-tag">Error</span><p>' +
      msg.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;') + '</p></div>';
  }
```

- [ ] **Step 4: Build and smoke-test**

```bash
go build ./...
```

Expected: build succeeds with no errors.

Start the server and open a browser:
```bash
./chemres
```

Navigate to `http://localhost:8080`, enter a few compound names in the batch textarea, click Resolve All. Verify:
- Progress bar appears and fills with real counts (e.g. "3 / 5")
- Result table appears when done
- No JavaScript errors in browser console

- [ ] **Step 5: Commit**

```bash
git add templates/index.html static/style.css
git commit -m "feat: batch progress bar with real SSE fill"
```

---

## Self-Review

**Spec coverage:**
- Job store (TTL 10min, `crypto/rand` IDs, sweep goroutine): Task 1 ✅
- `BatchWithProgress` on Resolver interface; `Batch` delegates: Task 2 ✅
- `/batch/start` POST → JSON job ID: Task 3 ✅
- `/batch/stream` SSE: `progress`, `done`, `error` events: Task 4 ✅
- Job context independent of HTTP request (IP attached separately): Task 3 (`resolver.WithClientIP(job.Ctx, ip)`) ✅
- Client disconnect doesn't cancel job; SSE handler stops streaming: Task 4 (`r.Context().Done()` exits SSE loop, not job) ✅
- `event: done` data: rendered `batch_result.html` with newlines collapsed: Task 4 ✅
- `drawStructure` called after DOM swap (replaces old `htmx:afterSettle`): Task 6 ✅
- CSS: `.progress-fill` with `transition: width 0.3s ease`: Task 6 ✅
- Preserve file upload path: Task 3 (copied from old batch.go) ✅
- Preserve retry logic / X-Forwarded-For / rate limiting: not touched — lives in `pubchemClient.do()` ✅

**Placeholder scan:** None found.

**Type consistency:**
- `jobs.Snapshot` used in Task 4 matches definition in Task 1 ✅
- `batchData` / `batchSummary` defined in `batch.go` (Task 3), consumed in `batchstream.go` (Task 4) — same package ✅
- `NewBatchStreamHandler(tmpl, resolver, store)` signature consistent across Task 4 definition and Task 5 usage ✅
- `stubResolver.BatchWithProgress` added in Task 2 satisfies interface for Task 3 tests ✅
