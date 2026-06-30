package resolver

import (
	"context"
	"log"
	"strings"
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

func (r *NameResolver) Resolve(ctx context.Context, input string) (CompoundResult, error) {
	return r.resolve(ctx, input, true)
}

func (r *NameResolver) resolve(ctx context.Context, input string, withSynonyms bool) (CompoundResult, error) {
	result := CompoundResult{Input: input, ResolvedAt: time.Now().UTC()}

	namespace := "name"
	postKey   := ""
	if inchiKeyRE.MatchString(input) {
		namespace = "inchikey"
	} else if strings.HasPrefix(input, "InChI=") {
		namespace = "inchi"
		postKey   = "inchi"
	}

	props, err := r.client.fetchProperties(ctx, namespace, input, postKey)
	if err == errNotFound {
		result.Error = "Not found in PubChem"
		if namespace == "name" {
			if suggestions, _ := r.client.autocomplete(ctx, input, 5); len(suggestions) > 0 {
				result.Suggestions = suggestions
			}
		}
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
	result.Formula             = p.MolecularFormula
	result.MW                  = p.MolecularWeight
	result.InChIKey             = p.InChIKey
	result.InChI               = p.InChI
	result.CommonName          = p.Title
	result.XLogP               = p.XLogP
	result.ExactMass           = p.ExactMass
	result.TPSA                = p.TPSA
	result.HBondDonorCount     = p.HBondDonorCount
	result.HBondAcceptorCount  = p.HBondAcceptorCount
	result.RotatableBondCount  = p.RotatableBondCount
	result.AtomStereoCount     = p.AtomStereoCount
	result.Charge              = p.Charge
	result.Volume3D            = p.Volume3D

	if withSynonyms {
		type synResult struct {
			cas  string
			syns []string
			err  error
		}
		type ghsResult struct {
			ghs *GHSData
			err error
		}

		synCh := make(chan synResult, 1)
		ghsCh := make(chan ghsResult, 1)

		go func() {
			cas, syns, err := r.client.fetchSynonyms(ctx, p.CID)
			synCh <- synResult{cas, syns, err}
		}()
		go func() {
			ghs, err := r.client.fetchGHS(ctx, p.CID)
			ghsCh <- ghsResult{ghs, err}
		}()

		sr := <-synCh
		if sr.err != nil {
			log.Printf("WARN fetchSynonyms cid=%d: %v", p.CID, sr.err)
		} else if sr.cas != "" || len(sr.syns) > 0 {
			result.CAS      = sr.cas
			result.Synonyms = sr.syns
			if result.CommonName == "" {
				result.CommonName = firstCommonName(sr.syns)
			}
		}

		gr := <-ghsCh
		if gr.err != nil {
			log.Printf("WARN fetchGHS cid=%d: %v", p.CID, gr.err)
		} else {
			result.GHS = gr.ghs
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
			res, err := r.resolve(ctx, in, true)
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
