package modelfingerprint

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type DistributionSample struct {
	ID             string             `json:"id"`
	Valid          int                `json:"valid"`
	Missing        int                `json:"missing"`
	Invalid        int                `json:"invalid"`
	Errors         int                `json:"errors"`
	ReferenceValid int                `json:"reference_valid"`
	Counts         map[string]int     `json:"counts"`
	Reference      map[string]float64 `json:"reference"`
	JSD            *float64           `json:"jsd,omitempty"`
}
type DistributionResult struct {
	Date    string               `json:"date"`
	Status  string               `json:"status"`
	MeanJSD *float64             `json:"mean_jsd,omitempty"`
	Cells   []DistributionSample `json:"cells"`
}

func numericDistributionAnswer(answer string, maximum int) string {
	answer = strings.TrimSpace(answer)
	answer = strings.Trim(answer, "\"“”'‘’")
	answer = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(answer), "."))
	// Unlike free-text cells these tasks require one integer; do not extract an
	// incidental number from explanations or refusals.
	n, err := strconv.Atoi(answer)
	if err != nil || n < 1 || n > maximum {
		return ""
	}
	return strconv.Itoa(n)
}
func evaluateDistribution(reference *distribution, observations map[string]Observation) *DistributionResult {
	result := &DistributionResult{Date: reference.Date, Status: "evidence_only", Cells: []DistributionSample{}}
	sum := 0.0
	count := 0
	allMetadata := []Observation{}
	for _, cell := range reference.Cells {
		sample := DistributionSample{ID: cell.ID, ReferenceValid: cell.Valid, Counts: map[string]int{}, Reference: cell.Dist}
		metadata := []Observation{}
		for i := 0; i < DistributionRepetitions; i++ {
			o, ok := observations[fmt.Sprintf("public_dist_%s_%d", cell.ID, i)]
			if !ok {
				sample.Missing++
				continue
			}
			if o.Error {
				sample.Errors++
				continue
			}
			value := numericDistributionAnswer(o.Answer, cell.Maximum)
			if value == "" || o.Truncated {
				sample.Invalid++
				continue
			}
			sample.Valid++
			sample.Counts[value]++
			metadata = append(metadata, o)
			allMetadata = append(allMetadata, o)
		}
		if sample.Valid >= 20 && consistentMetadata(metadata) {
			measured := map[string]float64{}
			for key, count := range sample.Counts {
				measured[key] = float64(count) / float64(sample.Valid)
			}
			jsd := JensenShannon(measured, cell.Dist)
			sample.JSD = &jsd
			sum += jsd
			count++
		}
		result.Cells = append(result.Cells, sample)
	}
	if count == len(reference.Cells) && count > 0 && consistentMetadata(allMetadata) {
		mean := sum / float64(count)
		result.MeanJSD = &mean
	} else {
		result.Status = "inconclusive"
	}
	return result
}

// JensenShannon normalizes rounded public probabilities; zero counts stay zero.
func JensenShannon(left, right map[string]float64) float64 {
	keys := map[string]bool{}
	l, r := 0.0, 0.0
	for k, v := range left {
		keys[k] = true
		l += v
	}
	for k, v := range right {
		keys[k] = true
		r += v
	}
	if l <= 0 || r <= 0 {
		return 0
	}
	sum := 0.0
	for _, key := range sortedKeys(keys) {
		p, q := left[key]/l, right[key]/r
		m := (p + q) / 2
		if p > 0 {
			sum += p * math.Log2(p/m) / 2
		}
		if q > 0 {
			sum += q * math.Log2(q/m) / 2
		}
	}
	return math.Max(0, math.Min(1, sum))
}
