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

	c := &pubchemClient{baseURL: srv.URL, http: srv.Client()}
	cas, syns, err := c.fetchSynonyms(180)
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
