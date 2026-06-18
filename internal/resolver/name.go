package resolver

import (
	"sync"
	"time"
)

type NameResolver struct {
	client *pubchemClient
}

func NewNameResolver() Resolver {
	return &NameResolver{client: newPubchemClient()}
}

func (r *NameResolver) SystemID() string { return "name" }
func (r *NameResolver) Name() string     { return "Name / CAS" }

func (r *NameResolver) Resolve(input string) (CompoundResult, error) {
	return r.resolve(input, true)
}

func (r *NameResolver) resolve(input string, fetchSVG bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	props, err := r.client.fetchProperties("name", input, false)
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

	if cas, _ := r.client.fetchCAS(p.CID); cas != "" {
		result.CAS = cas
	}
	if fetchSVG {
		if svg, _ := r.client.fetchSVG(p.CID); svg != "" {
			result.SVG = svg
		}
	}
	return result, nil
}

func (r *NameResolver) Batch(inputs []string) ([]CompoundResult, error) {
	results := make([]CompoundResult, len(inputs))
	sem := make(chan struct{}, batchWorkers)
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, in string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res, err := r.resolve(in, false)
			if err != nil {
				res = CompoundResult{Input: in, Error: "API error: " + err.Error(), ResolvedAt: time.Now().UTC()}
			}
			results[idx] = res
		}(i, input)
	}
	wg.Wait()
	return results, nil
}
