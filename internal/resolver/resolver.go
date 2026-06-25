package resolver

import (
	"context"
	"time"
)

type CompoundResult struct {
	Input      string    `json:"input"`
	CID        int       `json:"cid"`
	IUPAC      string    `json:"iupac"`
	Canonical  string    `json:"canonical_smiles"`
	Isomeric   string    `json:"isomeric_smiles"`
	Formula    string    `json:"formula"`
	MW         string    `json:"mw"`
	InChIKey   string    `json:"inchikey"`
	CAS        string    `json:"cas"`
	CommonName string    `json:"common_name"`
	Synonyms   []string  `json:"synonyms"`
	ResolvedAt time.Time `json:"resolved_at"`
	Error      string    `json:"error,omitempty"`
}

type Resolver interface {
	SystemID() string
	Resolve(ctx context.Context, input string) (CompoundResult, error)
	Batch(ctx context.Context, inputs []string) ([]CompoundResult, error)
	BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error)
	Suggest(ctx context.Context, query string) ([]string, error)
}
