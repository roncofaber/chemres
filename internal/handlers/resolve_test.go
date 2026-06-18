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
