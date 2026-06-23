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
