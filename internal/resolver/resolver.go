package resolver

import (
	"context"
	"html/template"
	"time"
)

type CompoundResult struct {
	Input      string
	CID        int
	IUPAC      string
	Canonical  string
	Isomeric   string
	Formula    string
	MW         string
	InChIKey   string
	CAS        string
	CommonName string
	Synonyms   []string
	SVG        template.HTML
	ResolvedAt time.Time
	Error      string
}

type Resolver interface {
	SystemID() string
	Name()     string
	Resolve(ctx context.Context, input string) (CompoundResult, error)
	Batch(ctx context.Context, inputs []string) ([]CompoundResult, error)
	BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error)
	Suggest(ctx context.Context, query string) ([]string, error)
}
