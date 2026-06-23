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
