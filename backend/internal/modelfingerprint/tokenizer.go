package modelfingerprint

import "fmt"

type TokenSample struct {
	ID        string `json:"id"`
	Reference int    `json:"reference"`
	Observed  *int64 `json:"observed,omitempty"`
	Status    string `json:"status"`
}
type TokenizerResult struct {
	Status   string        `json:"status"`
	Provider string        `json:"provider"`
	Date     string        `json:"date"`
	ProbeSet string        `json:"probe_set"`
	Samples  []TokenSample `json:"samples"`
}

func evaluateTokenizer(reference *tokenizer, observations map[string]Observation) *TokenizerResult {
	result := &TokenizerResult{Status: "consistent", Provider: reference.Provider, Date: reference.Date, ProbeSet: reference.ProbeSet, Samples: []TokenSample{}}
	allSamples := []Observation{}
	for _, key := range sortedKeys(reference.Values) {
		sample := TokenSample{ID: key, Reference: reference.Values[key], Status: "inconclusive"}
		samples := []Observation{}
		for i := 0; i < 3; i++ {
			samples = append(samples, observations[fmt.Sprintf("public_%s_%d", key, i)])
		}
		allSamples = append(allSamples, samples...)
		a, b, base := samples[0].InputTokens, samples[1].InputTokens, samples[2].InputTokens
		if consistentMetadata(samples) && a != nil && b != nil && base != nil && *a > 0 && *b > 0 && *base > 0 {
			if *a != *b {
				sample.Status = "unstable"
			} else if *a >= *base {
				delta := *a - *base
				sample.Observed = &delta
				sample.Status = "consistent"
				if delta != int64(sample.Reference) {
					sample.Status = "different"
				}
			}
		}
		result.Samples = append(result.Samples, sample)
		if sample.Status == "different" {
			result.Status = "different"
		} else if sample.Status != "consistent" && result.Status != "different" {
			result.Status = "inconclusive"
		}
	}
	if !consistentMetadata(allSamples) {
		result.Status = "inconclusive"
	}
	return result
}
