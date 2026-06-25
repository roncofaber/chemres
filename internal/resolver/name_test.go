package resolver

import (
	"context"
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

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, autocompleteBase: srv.URL, http: srv.Client()}}
	got, err := r.Resolve(context.Background(), "acetone")
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
}

func TestNameResolver_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer srv.Close()

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, autocompleteBase: srv.URL, http: srv.Client()}}
	got, err := r.Resolve(context.Background(), "notacompound12345xyz")
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

	r := &NameResolver{client: &pubchemClient{baseURL: srv.URL, autocompleteBase: srv.URL, http: srv.Client()}}
	results, err := r.Batch(context.Background(), []string{"acetone", "acetone"})
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
	}
}
