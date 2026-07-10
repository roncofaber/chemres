package resolver

import (
	"context"
	"time"
)

type BatchOpts struct {
	Synonyms bool
	GHS      bool
}

type batchOptsKey struct{}

func WithBatchOpts(ctx context.Context, opts BatchOpts) context.Context {
	return context.WithValue(ctx, batchOptsKey{}, opts)
}

func batchOptsFrom(ctx context.Context) BatchOpts {
	if opts, ok := ctx.Value(batchOptsKey{}).(BatchOpts); ok {
		return opts
	}
	return BatchOpts{Synonyms: true}
}

type GHSData struct {
	Signal      string   `json:"signal"`
	Pictograms  []string `json:"pictograms"`
	HStatements []string `json:"h_statements"`
	PCodes      string   `json:"p_codes,omitempty"`
}

type CompoundResult struct {
	Input      string    `json:"input"`
	CID        int       `json:"cid"`
	IUPAC      string    `json:"iupac"`
	Canonical  string    `json:"canonical_smiles"`
	Isomeric   string    `json:"isomeric_smiles"`
	Formula    string    `json:"formula"`
	MW         string    `json:"mw"`
	InChIKey   string    `json:"inchikey"`
	InChI      string    `json:"inchi,omitempty"`
	CAS        string    `json:"cas"`
	CommonName string    `json:"common_name"`
	Synonyms    []string  `json:"synonyms"`
	Suggestions []string  `json:"suggestions,omitempty"`
	GHS         *GHSData  `json:"ghs,omitempty"`
	Role        string    `json:"role,omitempty"`
	ResolvedAt  time.Time `json:"resolved_at"`
	Error       string    `json:"error,omitempty"`
	// Computed physicochemical properties
	XLogP              *float64 `json:"xlogp,omitempty"`
	ExactMass          string   `json:"exact_mass,omitempty"`
	TPSA               *float64 `json:"tpsa,omitempty"`
	HBondDonorCount    *int     `json:"hbond_donor_count,omitempty"`
	HBondAcceptorCount *int     `json:"hbond_acceptor_count,omitempty"`
	RotatableBondCount *int     `json:"rotatable_bond_count,omitempty"`
	AtomStereoCount    *int     `json:"atom_stereo_count,omitempty"`
	Charge             *int     `json:"charge,omitempty"`
	Volume3D           *float64 `json:"volume_3d,omitempty"`
}

type Resolver interface {
	SystemID() string
	Resolve(ctx context.Context, input string) (CompoundResult, error)
	Batch(ctx context.Context, inputs []string) ([]CompoundResult, error)
	BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error)
	Suggest(ctx context.Context, query string) ([]string, error)
}
