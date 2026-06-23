# PubChem Rate Limiting, IP Forwarding & Retry Design

**Date:** 2026-06-22
**Status:** Approved

## Background

PubChem enforces a 5 req/s limit per connecting IP. When deployed, all traffic originates from the server IP, so all users share one budget. Empirical testing (100 parallel requests) confirmed that PubChem honours `X-Forwarded-For` for rate limiting: with unique spoofed IPs, 100/100 requests succeeded; without, 85–90% returned 503.

This means each user's real IP can be forwarded to give them their own independent 5 req/s quota.

## Goals

1. Forward the real client IP via `X-Forwarded-For` on all outgoing PubChem requests.
2. Apply a per-client-IP token bucket (4 req/s) inside the server to keep each user safely under PubChem's limit.
3. Retry transient errors (503, network timeout) with exponential backoff before surfacing them.

## Architecture: Everything in the Client Layer (Option A)

All three concerns live in `pubchemClient`. The user IP is threaded via `context.Context` from handler → resolver → client. This keeps resolvers and the batch fan-out logic unchanged except for passing context through.

### Context key & IP extraction

A private unexported key type carries the client IP through context:

```
type ctxKey int
const clientIPKey ctxKey = 0

func WithClientIP(ctx context.Context, ip string) context.Context
func clientIPFromCtx(ctx context.Context) string
```

Handlers extract the real IP (preferring `X-Forwarded-For`, falling back to `RemoteAddr`) and attach it with `WithClientIP` before calling into the resolver.

### X-Forwarded-For forwarding

Every outgoing `http.Request` built by `pubchemClient` gets the IP from context set as the `X-Forwarded-For` header. If no IP is present in context (e.g. in tests), the header is omitted.

### Per-IP rate limiter

`pubchemClient` holds a `sync.Map` from client IP string → `*rate.Limiter`. On each request it looks up or creates a limiter for that IP (`rate.NewLimiter(4, 4)` — 4 tokens/s, burst of 4). It calls `limiter.Wait(ctx)` before issuing the HTTP call. This blocks the goroutine until a token is available, providing natural queuing at no extra cost.

Rate: **4 req/s** (safely under PubChem's 5/s).
Burst: **4** (a small batch starts immediately; sustained throughput throttles to 4/s).

Limiter map entries are never evicted (low cardinality in practice — one entry per unique user IP seen during server lifetime).

### Retry with exponential backoff

Transient errors (503 Service Unavailable, `context.DeadlineExceeded`, `net.Error` with `Timeout()`) are retried up to **3 times** with backoff: 250ms, 500ms, 1000ms. Non-transient errors (404, 400, other 4xx) are never retried.

Each retry logs at `WARN` level:
```
WARN pubchem retry attempt=1 status=503 url=... ip=...
```

After exhausting retries, the error is returned as-is to the resolver, which surfaces it to the user as "Temporary PubChem error — please try again."

### Resolver & Batch changes

The `Resolver` interface gains a context-aware variant used internally:

```go
// Internal resolve methods take a context
resolve(ctx context.Context, input string, fetchSVG bool) (CompoundResult, error)
```

Public `Resolve(input string)` calls `resolve(context.Background(), input, true)` for backwards compatibility. `Batch` receives the context from the handler and passes it to each worker goroutine.

The `Batch` method in `AutoResolver` (and unused ones in `NameResolver`/`SmilesResolver`) retains the concurrency semaphore (`batchWorkers=5`) alongside the rate limiter — the semaphore caps goroutine count, the limiter caps throughput. These are complementary.

### Handler changes

- `ResolveHandler`, `BatchHandler`, `SuggestHandler`: extract client IP, call `WithClientIP`, pass context to resolver calls.
- No changes to template rendering or response structure.

## Data Flow

```
HTTP request (client IP: 1.2.3.4)
  → Handler: extracts IP, wraps in ctx
  → Resolver.Batch(ctx, inputs)
    → for each input, goroutine:
        → pubchemClient.fetchProperties(ctx, ...)
            → limiter[1.2.3.4].Wait(ctx)   // blocks if >4/s
            → build http.Request + X-Forwarded-For: 1.2.3.4
            → retry loop (up to 3x on transient error)
            → return result
```

## Error Classification

| PubChem response | Transient? | Action |
|---|---|---|
| 503 Service Unavailable | Yes | Retry with backoff |
| Network timeout | Yes | Retry with backoff |
| 404 Not Found | No | Return `errNotFound` |
| 400 Bad Request | No | Return `errBadInput` |
| Other 4xx | No | Return error immediately |
| 5xx (not 503) | Yes | Retry with backoff |

## Dependencies

Add `golang.org/x/time/rate` (already in the Go extended stdlib, zero new external deps).

## Testing

- `client_test.go`: unit tests for IP extraction from context, `X-Forwarded-For` header presence, retry behaviour (mock HTTP server returning 503 then 200).
- `auto_test.go`: batch test verifying per-IP limiter is keyed correctly.
- Existing integration tests remain unchanged.
