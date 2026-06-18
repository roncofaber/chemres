package resolver

import (
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
	SVG        template.HTML
	ResolvedAt time.Time
	Error      string
}

type Resolver interface {
	SystemID() string
	Name()     string
	Resolve(input string) (CompoundResult, error)
	Batch(inputs []string) ([]CompoundResult, error)
}
