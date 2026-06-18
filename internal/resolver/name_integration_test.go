//go:build integration

package resolver

import "testing"

func TestNameResolver_Integration_CommonName(t *testing.T) {
	r := NewNameResolver()
	got, err := r.Resolve("acetone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.Canonical == "" {
		t.Error("expected non-empty Canonical SMILES")
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
}

func TestNameResolver_Integration_CAS(t *testing.T) {
	r := NewNameResolver()
	got, err := r.Resolve("67-64-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
}
