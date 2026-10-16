package modelfingerprint

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

type KnowledgeResult struct {
	Status            string   `json:"status"`
	Reasons           []string `json:"reasons,omitempty"`
	FailedRequests    int      `json:"failed_requests"`
	MissingRequests   int      `json:"missing_requests"`
	TruncatedRequests int      `json:"truncated_requests"`
	Provider          string   `json:"provider"`
	Date              string   `json:"date"`
	Total             int      `json:"total"`
	Valid             int      `json:"valid"`
	Mismatches        int      `json:"mismatches"`
	SelfTotal         int      `json:"self_total"`
	SelfMismatches    int      `json:"self_mismatches"`
	BaselineUpper     *float64 `json:"baseline_upper,omitempty"`
	PValue            *float64 `json:"p_value,omitempty"`
}

var indexedNumber = regexp.MustCompile(`^(?:\(([0-9]+)\)|([0-9]+)[.)])\s*(.*)$`)
var numericAnswer = regexp.MustCompile(`^-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// ParseKnowledgeAnswers is deliberately stricter than upstream's last-number
// extraction. Ambiguous explanations, duplicate indices and unnumbered output
// are missing evidence, never a wrong answer or a match.
func ParseKnowledgeAnswers(answer string, n int, lower, upper float64) []*float64 {
	values := make([]*float64, n)
	seen := map[int]bool{}
	for _, line := range strings.Split(strings.TrimSpace(answer), "\n") {
		match := indexedNumber.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		index := match[1]
		if index == "" {
			index = match[2]
		}
		i, err := strconv.Atoi(index)
		if err != nil || i < 1 || i > n {
			continue
		}
		i--
		if seen[i] {
			return make([]*float64, n)
		}
		seen[i] = true
		raw := strings.NewReplacer("−", "-", "–", "-", ",", "").Replace(strings.TrimSpace(match[3]))
		if !numericAnswer.MatchString(raw) {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lower && v <= upper {
			values[i] = &v
		}
	}
	return values
}
func KnowledgeMatches(domainID string, reference, observed float64) bool {
	d, ok := data.Domains[domainID]
	if !ok || math.IsNaN(reference) || math.IsNaN(observed) || math.IsInf(reference, 0) || math.IsInf(observed, 0) {
		return false
	}
	if d.Mode == "absolute" {
		return math.Abs(math.Round(reference)-math.Round(observed)) <= d.Tolerance
	}
	return math.Abs(reference-observed)/math.Max(math.Abs(reference), 1e-10) <= d.Tolerance
}
func evaluateKnowledge(k *knowledge, plan []Probe, observations map[string]Observation) *KnowledgeResult {
	result := &KnowledgeResult{Status: "inconclusive", Provider: k.Provider, Date: k.Date, Total: len(k.Questions), SelfTotal: k.SelfTotal, SelfMismatches: k.SelfMismatches}
	metadata := []Observation{}
	for _, probe := range plan {
		if probe.Kind != "knowledge" {
			continue
		}
		o, ok := observations[probe.ID]
		if !ok {
			result.MissingRequests++
			continue
		}
		if o.Error {
			result.FailedRequests++
			continue
		}
		if o.Truncated {
			result.TruncatedRequests++
			continue
		}
		metadata = append(metadata, o)
		d := data.Domains[probe.Questions[0].Domain]
		values := ParseKnowledgeAnswers(o.Answer, len(probe.Questions), d.Range[0], d.Range[1])
		for i, value := range values {
			if value == nil {
				continue
			}
			result.Valid++
			if !KnowledgeMatches(probe.Questions[i].Domain, probe.Questions[i].Value, *value) {
				result.Mismatches++
			}
		}
	}
	result.Reasons = knowledgeBlockingReasons(result, metadata)
	if len(result.Reasons) > 0 {
		return result
	}
	upper := ClopperPearsonUpper(k.SelfMismatches, k.SelfTotal)
	p := BinomialTail(result.Mismatches, result.Valid, math.Min(upper, .99))
	result.BaselineUpper = &upper
	result.PValue = &p
	result.Status = "matched"
	if p < .05 {
		result.Status = "mismatch"
	}
	return result
}

// BinomialTail returns P(X >= k) without subtracting nearly equal CDF values.
func BinomialTail(k, n int, p float64) float64 {
	if k <= 0 {
		return 1
	}
	if k > n || p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	total := 0.0
	lnN, _ := math.Lgamma(float64(n + 1))
	for i := k; i <= n; i++ {
		lnI, _ := math.Lgamma(float64(i + 1))
		lnOther, _ := math.Lgamma(float64(n - i + 1))
		total += math.Exp(lnN - lnI - lnOther + float64(i)*math.Log(p) + float64(n-i)*math.Log1p(-p))
	}
	return math.Min(1, total)
}

// ClopperPearsonUpper inverts the binomial tail at one-sided confidence .99.
func ClopperPearsonUpper(k, n int) float64 {
	if n <= 0 || k >= n {
		return 1
	}
	low, high := 0.0, 1.0
	for i := 0; i < 60; i++ {
		middle := (low + high) / 2
		if BinomialTail(k+1, n, middle) < .99 {
			low = middle
		} else {
			high = middle
		}
	}
	return (low + high) / 2
}
