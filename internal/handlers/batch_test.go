package handlers

import (
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
