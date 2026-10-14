package animationquality

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// Python's str.isspace set includes U+001C..001F, which Go's unicode.IsSpace
// omits. Locked to the Python Unicode whitespace contract of sklearn-char-v1.
func pythonSpace(r rune) bool {
	return r >= 0x09 && r <= 0x0d || r >= 0x1c && r <= 0x20 || r == 0x85 || r == 0xa0 || r == 0x1680 || r >= 0x2000 && r <= 0x200a || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}
func preprocess(source string) []rune {
	input := []rune(source)
	out := make([]rune, 0, len(input))
	for i := 0; i < len(input); {
		end := i + 1
		if pythonSpace(input[i]) {
			for end < len(input) && pythonSpace(input[end]) {
				end++
			}
		}
		if end-i >= 2 {
			out = append(out, ' ')
		} else {
			out = append(out, input[i])
		}
		i = end
	}
	return out
}

func (d *Detector) Predict(prompt, source string) Prediction {
	result := Prediction{Status: "unknown", Benchmark: Benchmark(prompt)}
	if !utf8.ValidString(source) || len(source) > MaxSourceBytes || strings.TrimSpace(source) == "" {
		result.Reason = "source_unavailable"
		return result
	}
	if prompt == "" {
		result.Reason = "prompt_unavailable"
		return result
	}
	if result.Benchmark != "pelican-v1" {
		result.Reason = "unsupported_benchmark"
		return result
	}
	if !strings.Contains(strings.ToLower(source), "<html") {
		result.Reason = "unsupported_format"
		return result
	}
	if d == nil {
		result.Reason = "model_unavailable"
		return result
	}
	result.ModelVersion, result.ModelSHA256 = d.model.ModelVersion, d.hash
	counts := map[int]float64{}
	text := preprocess(source)
	for n := d.model.NgramMin; n <= d.model.NgramMax; n++ {
		for i := 0; i+n <= len(text); i++ {
			if index, ok := d.model.Vocabulary[string(text[i:i+n])]; ok {
				counts[index]++
			}
		}
	}
	if len(counts) == 0 {
		result.Reason = "no_known_features"
		return result
	}
	indices := make([]int, 0, len(counts))
	for i := range counts {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	norm := 0.0
	for _, i := range indices {
		counts[i] *= d.model.IDF[i]
		norm = math.Hypot(norm, counts[i])
	}
	z := d.model.Intercept
	for _, i := range indices {
		z += d.model.Weights[i] * (counts[i] / norm)
	}
	p := 0.0
	if z >= 0 {
		p = 1 / (1 + math.Exp(-z))
	} else {
		e := math.Exp(z)
		p = e / (1 + e)
	}
	result.Score = &p
	if p < d.model.Threshold-d.model.UnknownMargin {
		result.Status = "normal"
	} else if p > d.model.Threshold+d.model.UnknownMargin {
		result.Status = "suspicious"
	} else {
		result.Reason = "ambiguous_score"
	}
	return result
}
