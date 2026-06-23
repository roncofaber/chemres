# Batch Progress Bar — SSE Design

**Date:** 2026-06-23
**Status:** Approved

## Background

Batch resolution takes ~25s for 100 compounds (rate-limited to 4 req/s for name→CID calls). The current UI shows an indeterminate sliding bar with no real feedback. This design adds genuine progress — "47 / 100 resolved" filling a bar — using Server-Sent Events.

## Goals

1. Show real progress as compounds resolve in phase 1 (name→CID+props).
2. Show the full result table once, complete, after phase 2 (synonyms) finishes.
3. Keep the batch running server-side if the client disconnects; resume streaming on reconnect.
4. Preserve all existing behaviour: retry logic, X-Forwarded-For forwarding, rate limiting, file upload, error messages.

## Architecture

### New files

| File | Responsibility |
|------|---------------|
| `internal/jobs/store.go` | `Job` struct, `JobStore`, TTL cleanup |
| `internal/handlers/batchstream.go` | SSE stream handler (`GET /batch/stream`) |

### Modified files

| File | Change |
|------|--------|
| `internal/resolver/auto.go` | Add `BatchWithProgress` method |
| `internal/handlers/batch.go` | POST handler creates job + returns JSON; existing logic stays |
| `main.go` | Register `/batch/start` and `/batch/stream` routes |
| `templates/index.html` | Replace htmx batch form with JS EventSource flow |
| `static/style.css` | Add `.progress-fill` real-fill styles |

## Job Store (`internal/jobs/store.go`)

```go
type Job struct {
    mu       sync.Mutex
    total    int
    done     int
    finished bool
    results  []resolver.CompoundResult
    err      error
    created  time.Time
    ctx      context.Context
    cancel   context.CancelFunc
}

type JobStore struct {
    jobs sync.Map // string → *Job
}
```

- Job IDs are 16 random bytes, hex-encoded (`crypto/rand`).
- `JobStore.New(total int) (id string, job *Job)` — creates job with its own independent context (NOT derived from any HTTP request context).
- `JobStore.Get(id string) (*Job, bool)` — looks up a job.
- A background goroutine (started once at server init) sweeps the store every minute and cancels+deletes jobs older than 10 minutes.

Job methods:
- `job.Incr()` — increments done count (called by batch goroutine after each compound).
- `job.Finish(results []resolver.CompoundResult, err error)` — marks finished.
- `job.Snapshot() (done, total int, finished bool, results, err)` — mutex-protected read.

## Resolver: BatchWithProgress

```go
func (r *AutoResolver) BatchWithProgress(
    ctx context.Context,
    inputs []string,
    onResolve func(done, total int),
) ([]CompoundResult, error)
```

Identical to `Batch` but calls `onResolve(done, total)` after each goroutine completes in phase 1. Phase 2 (batch synonyms) is a single call with no per-call callback. `onResolve` increments the job's done counter.

`Batch` is retained unchanged and delegates to `BatchWithProgress` with a no-op callback:

```go
func (r *AutoResolver) Batch(ctx context.Context, inputs []string) ([]CompoundResult, error) {
    return r.BatchWithProgress(ctx, inputs, func(_, _ int) {})
}
```

## HTTP Handlers

### POST `/batch/start`

Replaces the current `POST /batch`. Parses inputs identically (text area or file upload). On validation error (empty, too large), returns JSON:

```json
{"error": "No valid inputs found — enter one per line."}
```

On success:
1. Creates a job via `store.New(len(inputs))`.
2. Launches a goroutine: calls `resolver.BatchWithProgress(job.ctx, inputs, job.Incr)`, then `job.Finish(results, err)`.
3. Returns `{"job": "<id>", "total": 47}`.

### GET `/batch/stream?job=<id>`

SSE handler. Sets headers:
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

- If job ID not found: returns 404 immediately.
- Polls job state every 200ms.
- On each poll, if `done` changed since last send: writes `event: progress\ndata: <done>/<total>\n\n` and flushes.
- When `finished`:
  - If `err != nil`: writes `event: error\ndata: <message>\n\n`, flushes, returns.
  - Otherwise: renders `batch_result.html` into a buffer, writes `event: done\ndata: <HTML>\n\n`, flushes, returns.
- Stops when client disconnects (`r.Context().Done()`). Job keeps running.

## SSE Event Protocol

```
event: progress
data: 15/100

event: progress
data: 100/100

event: done
data: <full rendered batch_result.html — may be large>
```

On error:
```
event: error
data: Could not reach PubChem — please try again.
```

The `done` event data is the full rendered HTML. The client swaps it directly into `#batch-result`.

## Frontend

The htmx `hx-post`, `hx-target`, `hx-indicator` attributes are removed from the batch form. The form keeps its existing structure; submission is intercepted by JS.

```javascript
function startBatch(e) {
    e.preventDefault();
    var form = document.getElementById('batch-form');
    var data = new FormData(form);

    // Show progress bar, reset state
    showBatchProgress(0, null);
    document.getElementById('batch-result').innerHTML = '';
    document.getElementById('batch-reset').style.display = 'flex';

    fetch('/batch/start', { method: 'POST', body: data })
        .then(function(r) { return r.json(); })
        .then(function(res) {
            if (res.error) { showBatchError(res.error); return; }
            var es = new EventSource('/batch/stream?job=' + res.job);
            var total = res.total;

            es.addEventListener('progress', function(ev) {
                var done = parseInt(ev.data.split('/')[0], 10);
                showBatchProgress(done / total, done + ' / ' + total);
            });
            es.addEventListener('done', function(ev) {
                es.close();
                hideBatchProgress();
                document.getElementById('batch-result').innerHTML = ev.data;
                document.querySelectorAll('.structure-wrap[data-smiles]').forEach(drawStructure);
            });
            es.addEventListener('error', function(ev) {
                es.close();
                hideBatchProgress();
                showBatchError(ev.data || 'Connection lost — please try again.');
            });
        })
        .catch(function() { showBatchError('Could not reach server.'); });
}
```

`showBatchProgress(fraction, label)` sets the `.progress-fill` width and updates the indicator text.
`hideBatchProgress()` hides the bar.
`showBatchError(msg)` renders the error into `#batch-result`.

## CSS

The `.progress-bar` gains a child `.progress-fill` for real width-based fill alongside the existing indeterminate animation (used by single lookup):

```css
.progress-fill {
    height: 100%;
    width: 0%;
    background: var(--accent);
    box-shadow: 0 0 8px var(--accent);
    transition: width 0.3s ease;
}
```

The batch JS sets `.progress-fill` width directly. The single-lookup indeterminate `::after` animation is unchanged.

## Error Handling

| Scenario | Behaviour |
|----------|-----------|
| Empty inputs / file too large | JSON error from `/batch/start`, no job created |
| PubChem unreachable (retries exhausted) | `event: error` on SSE stream |
| Invalid / expired job ID | 404 from `/batch/stream` |
| Client disconnects mid-batch | SSE handler stops; job context is independent, batch keeps running |
| Client reconnects | EventSource auto-reconnects; stream resumes from current progress |
| Job TTL expires (10 min) | Job cancelled and removed; reconnecting client gets 404 |

## Retry Compatibility

The existing retry logic in `pubchemClient.do()` is fully preserved. Retries abort cleanly when the job context is cancelled (TTL expiry), but not on client disconnect since the job context is independent of the HTTP request context.

## Routes

```go
mux.Handle("/batch/start",  handlers.NewBatchStartHandler(tmpl, r, store))
mux.Handle("/batch/stream", handlers.NewBatchStreamHandler(tmpl, store))
// Remove or keep /batch for backward compat — remove it (no external callers)
```

## Testing

| File | What it tests |
|------|--------------|
| `internal/jobs/store_test.go` | Job creation, `Incr`, `Finish`, `Snapshot`, TTL expiry |
| `internal/handlers/batch_test.go` | `/batch/start` returns job ID and total; validation errors |
| `internal/handlers/batchstream_test.go` | 404 for unknown job; correct event sequence for a mock finished job; error event for failed job |
| `internal/resolver/auto_test.go` | `BatchWithProgress` calls onResolve the correct number of times |
