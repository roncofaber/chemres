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
