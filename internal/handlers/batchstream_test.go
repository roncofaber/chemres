package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
