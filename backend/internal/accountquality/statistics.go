package accountquality

import (
	"math"
	"time"
)

type Window struct {
	Score        *float64 `json:"score"`
	Samples      int      `json:"samples"`
	Passed       int      `json:"passed"`
	Failed       int      `json:"failed"`
	Inconclusive int      `json:"inconclusive"`
}

type Statistics struct {
	Short       Window `json:"short"`
	Long        Window `json:"long"`
	EvaluatedAt string `json:"evaluated_at"`
}

func New(now time.Time) Statistics {
	return Statistics{EvaluatedAt: now.UTC().Format(time.RFC3339Nano)}
}

// A sample is a deduplicated request or an account/model check, not a token or group membership.
func (s *Statistics) Add(now, at time.Time, outcome string) {
	if at.After(now) || at.Before(now.Add(-30*24*time.Hour)) {
		return
	}
	add(&s.Long, outcome)
	if !at.Before(now.Add(-24 * time.Hour)) {
		add(&s.Short, outcome)
	}
}

func add(w *Window, outcome string) {
	w.Samples++
	switch outcome {
	case "passed":
		w.Passed++
	case "failed":
		w.Failed++
	default:
		w.Inconclusive++
	}
}

func (s *Statistics) Calculate(confidence bool) {
	for _, w := range []*Window{&s.Short, &s.Long} {
		n := float64(w.Passed + w.Failed)
		if n == 0 {
			continue
		}
		value := float64(w.Passed) / n
		if confidence {
			// Wilson lower bound at 95% confidence; one match is not certainty of model identity.
			const z = 1.959963984540054
			value = (value + z*z/(2*n) - z*math.Sqrt(value*(1-value)/n+z*z/(4*n*n))) / (1 + z*z/n)
		}
		score := math.Round(math.Max(0, value)*1000) / 10
		w.Score = &score
	}
}
