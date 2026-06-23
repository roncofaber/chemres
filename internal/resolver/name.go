package resolver

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

var inchiKeyRE = regexp.MustCompile(`^[A-Z]{14}-[A-Z]{10}-[A-Z]$`)

type NameResolver struct {
	client *pubchemClient
}

func NewNameResolver() Resolver {
	return &NameResolver{client: newPubchemClient()}
}

func (r *NameResolver) SystemID() string { return "name" }
func (r *NameResolver) Name() string     { return "Name / CAS" }

func (r *NameResolver) Resolve(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true, true)
}

func (r *NameResolver) resolve(ctx context.Context, input string, fetchSVG bool, withSynonyms bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	namespace := "name"
	if inchiKeyRE.MatchString(input) {
		namespace = "inchikey"
	}

	props, err := r.client.fetchProperties(ctx, namespace, input, false)
	if err == errNotFound {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	if err == errBadInput {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(props.PropertyTable.Properties) == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}

	p := props.PropertyTable.Properties[0]
	if p.CID == 0 {
		result.Error = "Not found in PubChem"
		return result, nil
	}
	result.CID       = p.CID
	result.IUPAC     = p.IUPACName
	result.Canonical = p.CanonicalSMILES
	if result.Canonical == "" {
		result.Canonical = p.SMILES
	}
	result.Isomeric = p.IsomericSMILES
	if result.Isomeric == "" {
		result.Isomeric = p.ConnectivitySMILES
	}
	result.Formula   = p.MolecularFormula
	result.MW        = p.MolecularWeight
	result.InChIKey  = p.InChIKey

	if withSynonyms {
		if cas, syns, _ := r.client.fetchSynonyms(ctx, p.CID); cas != "" || len(syns) > 0 {
			result.CAS        = cas
			result.Synonyms   = syns
			result.CommonName = firstCommonName(syns)
		}
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(ctx, p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
}

func (r *NameResolver) Suggest(ctx context.Context, query string) ([]string, error) {
	if len(strings.TrimSpace(query)) < 2 {
		return nil, nil
	}
	return r.client.autocomplete(ctx, query, 10)
}

func (r *NameResolver) Batch(ctx context.Context, inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(ctx, in, false, true)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}

func (r *NameResolver) BatchWithProgress(ctx context.Context, inputs []string, onResolve func(done, total int)) ([]CompoundResult, error) {
	return r.Batch(ctx, inputs)
}
