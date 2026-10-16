package modelfingerprint

import (
	"fmt"
	"strings"
)

const DistributionRepetitions = 30

type Probe struct {
	ID              string
	Label           string
	Kind            string
	Prompt          string
	Instructions    string
	Temperature     *float64
	MaxOutputTokens int
	Questions       []Question
	Cell            string
}

// Plan preserves KBF's sorted domains, within-domain order, ten-question batches
// and exact templates. No retries or reference-driven answer selection are used.
func Plan(model string) []Probe {
	p := lookup(model)
	if p == nil {
		return nil
	}
	result := []Probe{}
	if p.KBF != nil {
		domains := map[string][]Question{}
		for _, q := range p.KBF.Questions {
			domains[q.Domain] = append(domains[q.Domain], q)
		}
		for _, key := range sortedKeys(domains) {
			questions := domains[key]
			for start := 0; start < len(questions); start += 10 {
				batch := append([]Question(nil), questions[start:min(start+10, len(questions))]...)
				sentences := []string{}
				for i, q := range batch {
					sentences = append(sentences, fmt.Sprintf("(%d) %s", i+1, strings.ReplaceAll(data.Domains[key].Template, "{name}", q.Name)))
				}
				temperature := 0.0
				result = append(result, Probe{ID: fmt.Sprintf("public_kbf_%s_%d", key, start/10), Label: fmt.Sprintf("知识边界 · %s · %d", key, start/10+1), Kind: "knowledge", Prompt: strings.ReplaceAll(data.Template, "{sentences}", strings.Join(sentences, "\n")), Instructions: p.KBF.System, Temperature: &temperature, MaxOutputTokens: 80 * len(batch), Questions: batch})
			}
		}
	}
	if p.Tokenizer != nil {
		for _, key := range sortedKeys(p.Tokenizer.Values) {
			for i, prompt := range []string{data.TokenPrompts[key], data.TokenPrompts[key], "a"} {
				result = append(result, Probe{ID: fmt.Sprintf("public_%s_%d", key, i), Label: fmt.Sprintf("公开分词 · %s · %d/3", key, i+1), Kind: "tokenizer", Prompt: prompt, MaxOutputTokens: 1, Cell: key})
			}
		}
	}
	if p.Distribution != nil {
		for round := 0; round < DistributionRepetitions; round++ {
			for _, cell := range p.Distribution.Cells {
				temperature := 1.0
				result = append(result, Probe{ID: fmt.Sprintf("public_dist_%s_%d", cell.ID, round), Label: fmt.Sprintf("参考分布 · %s · %d/30", cell.ID, round+1), Kind: "distribution", Prompt: cell.Prompt, Temperature: &temperature, MaxOutputTokens: 16, Cell: cell.ID})
			}
		}
	}
	return result
}
