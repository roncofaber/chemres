//go:build integration

package resolver

import (
	"context"
	"testing"
)

func TestSmilesResolver_Integration_Acetone(t *testing.T) {
	r := NewSmilesResolver()
	got, err := r.Resolve(context.Background(), "CC(C)=O")
	if err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatalf("unexpected error: %s", got.Error)
	}
	if got.IUPAC != "propan-2-one" {
		t.Errorf("IUPAC: got %q, want %q", got.IUPAC, "propan-2-one")
	}
	if got.CAS != "67-64-1" {
		t.Errorf("CAS: got %q, want %q", got.CAS, "67-64-1")
	}
	if got.Canonical == "" {
		t.Error("expected non-empty Canonical SMILES")
	}
}

func TestSmilesResolver_Integration_NotFound(t *testing.T) {
	r := NewSmilesResolver()
	_, err := r.Resolve(context.Background(), "XXXXXXXXXX")
	if err == nil {
		t.Fatal("expected error for invalid SMILES")
	}
}
