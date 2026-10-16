package modelfingerprint_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
)

func TestCatalogCoversAllSelectedVariantsWithoutGuessingAliases(t *testing.T) {
	catalog := modelfingerprint.Catalog()
	if len(catalog) != 18 {
		t.Fatalf("got %d variants", len(catalog))
	}
	for _, entry := range catalog {
		if entry.Model == "google/gemini-3.7-flash" {
			if entry.Requests != 0 || entry.Unavailable == "" {
				t.Fatal("missing Gemini data must not become usable reference")
			}
			continue
		}
		if entry.Requests != len(modelfingerprint.Plan(entry.Model)) {
			t.Fatal("budget differs from plan")
		}
		if entry.Requests <= 0 || len(entry.Methods) == 0 {
			t.Fatalf("missing plan: %+v", entry)
		}
	}
	for _, model := range []string{"claude-opus-5-5", "unknown/gpt-6-astra", "gpt-6-astra-20261016", "GPT-6-ASTRA", "deepseek-v4-pro"} {
		if len(modelfingerprint.Plan(model)) != 0 {
			t.Fatalf("unregistered alias guessed: %s", model)
		}
	}
	if len(modelfingerprint.Plan("gpt-6-astra")) != len(modelfingerprint.Plan("openai/gpt-6-astra")) {
		t.Fatal("explicit alias lost")
	}
}
func TestKnowledgePlanPreservesReferencePromptOrderAndBatchBudget(t *testing.T) {
	plan := modelfingerprint.Plan("gpt-6-astra")
	total := 0
	for _, p := range plan {
		if p.Kind != "knowledge" {
			continue
		}
		total += len(p.Questions)
		if p.Temperature == nil || *p.Temperature != 0 || p.MaxOutputTokens != 80*len(p.Questions) || len(p.Questions) > 10 {
			t.Fatalf("wrong parameters: %+v", p)
		}
		if !strings.HasPrefix(p.Prompt, "TASK: Answer these factual recall questions") || p.Instructions != "Follow the user's instructions exactly. Output only what is requested." {
			t.Fatal("prompt changed")
		}
	}
	if total != 538 {
		t.Fatalf("lost questions: %d", total)
	}
	if !strings.Contains(plan[0].ID, "astronomy") {
		t.Fatal("domain order changed")
	}
}
func answerReference(model string, replace bool) []modelfingerprint.Observation {
	result := []modelfingerprint.Observation{}
	for _, p := range modelfingerprint.Plan(model) {
		if p.Kind != "knowledge" {
			continue
		}
		lines := []string{}
		for i, q := range p.Questions {
			v := q.Value
			if replace {
				v = 42
			}
			lines = append(lines, fmt.Sprintf("(%d) %.12g", i+1, v))
		}
		result = append(result, modelfingerprint.Observation{ID: p.ID, Answer: strings.Join(lines, "\n"), Protocol: "chat_completions", ResponseModel: model})
	}
	return result
}
func TestKnowledgeComparisonUsesSelfBaselineAndSeparatesInconclusiveEvidence(t *testing.T) {
	for _, scenario := range []string{"consistent", "substituted", "missing", "failed", "mixed", "empty", "oauth"} {
		t.Run(scenario, func(t *testing.T) {
			obs := answerReference("gpt-6-astra", scenario == "substituted")
			why := ""
			switch scenario {
			case "missing":
				obs = obs[:len(obs)-1]
			case "failed":
				obs[0].Error = true
			case "mixed":
				obs[0].ResponseModel = "other"
			case "empty":
				for i := range obs {
					obs[i].Answer = "cannot answer"
				}
			case "oauth":
				why = "parameters incompatible"
			}
			r := modelfingerprint.Evaluate("gpt-6-astra", obs, why)
			want := "inconclusive"
			if scenario == "consistent" {
				want = "matched"
			}
			if scenario == "substituted" {
				want = "mismatch"
			}
			if r.Status != want {
				t.Fatalf("got %s: %+v", r.Status, r.Knowledge)
			}
			if scenario == "consistent" && (r.Knowledge.Valid < 530 || r.Knowledge.PValue == nil || *r.Knowledge.PValue != 1) {
				t.Fatal("reference answers must pass calibrated comparison")
			}
			if scenario == "substituted" && (r.Knowledge.PValue == nil || *r.Knowledge.PValue >= .05) {
				t.Fatal("substitution must exceed self error")
			}
		})
	}
}
func TestKnowledgeParserDoesNotGuessAmbiguousOrDuplicateAnswers(t *testing.T) {
	cases := []struct {
		answer string
		valid  int
	}{{"(2) 4\n(1) -2.5e1", 2}, {"(1) 1\n(1) 2", 0}, {"(1) between 1 and 2", 0}, {"(1) NaN\n(2) 1e999", 0}, {"(1) 2\n(2) 99999", 1}, {"1\n2", 0}}
	for _, tc := range cases {
		t.Run(tc.answer, func(t *testing.T) {
			values := modelfingerprint.ParseKnowledgeAnswers(tc.answer, 2, -100, 100)
			n := 0
			for _, v := range values {
				if v != nil {
					n++
				}
			}
			if n != tc.valid {
				t.Fatalf("got %d valid", n)
			}
		})
	}
	if !modelfingerprint.KnowledgeMatches("biology", 16.5, 17) || modelfingerprint.KnowledgeMatches("biology", 16.5, 16) {
		t.Fatal("half-away rounding changed")
	}
}
func TestBinomialStatisticsMatchKnownProbabilities(t *testing.T) {
	if math.Abs(modelfingerprint.BinomialTail(2, 3, .5)-.5) > 1e-12 {
		t.Fatal("wrong upper tail")
	}
	if math.Abs(modelfingerprint.ClopperPearsonUpper(0, 100)-(1-math.Pow(.01, .01))) > 1e-12 {
		t.Fatal("wrong one-sided 99% bound")
	}
	if modelfingerprint.ClopperPearsonUpper(100, 100) != 1 || modelfingerprint.BinomialTail(0, 100, .1) != 1 {
		t.Fatal("degenerate statistics")
	}
	if p := modelfingerprint.BinomialTail(80, 100, .1); p <= 0 || p >= 1e-50 {
		t.Fatalf("small tail lost precision: %g", p)
	}
}
func TestTokenizerCannotAuthenticateAndRejectsUnstableOrMissingUsage(t *testing.T) {
	for _, scenario := range []string{"consistent", "different", "unstable", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			obs := []modelfingerprint.Observation{}
			// Qwen3.7 Flash's pinned reference deltas, independently readable from modelprint.
			deltas := map[string]int64{"tok-english": 47, "tok-chinese": 51, "tok-code": 63, "tok-emoji": 92}
			for _, p := range modelfingerprint.Plan("qwen3.7-flash") {
				tokens := deltas[p.Cell] + 7
				if p.Prompt == "a" {
					tokens = 7
				}
				if scenario == "different" && p.Prompt != "a" {
					tokens++
				}
				if scenario == "unstable" && strings.HasSuffix(p.ID, "_1") {
					tokens++
				}
				o := modelfingerprint.Observation{ID: p.ID, InputTokens: &tokens, Protocol: "chat_completions", ResponseModel: "qwen3.7-flash"}
				if scenario == "missing" {
					o.InputTokens = nil
				}
				obs = append(obs, o)
			}
			r := modelfingerprint.Evaluate("qwen3.7-flash", obs, "")
			if r.Status != "inconclusive" {
				t.Fatal("tokenizer is not identity evidence")
			}
			want := "inconclusive"
			if scenario == "consistent" || scenario == "different" {
				want = scenario
			}
			if r.Tokenizer.Status != want {
				t.Fatalf("got %s", r.Tokenizer.Status)
			}
		})
	}
}
func TestDistributionShowsDistanceWithoutUncalibratedIdentityVerdict(t *testing.T) {
	obs := []modelfingerprint.Observation{}
	for _, p := range modelfingerprint.Plan("qwen3.7-max") {
		obs = append(obs, modelfingerprint.Observation{ID: p.ID, Answer: "7", Protocol: "chat_completions", ResponseModel: "qwen3.7-max"})
	}
	r := modelfingerprint.Evaluate("qwen3.7-max", obs, "")
	if r.Status != "inconclusive" || r.Distribution.MeanJSD == nil || len(r.Distribution.Cells) != 8 {
		t.Fatalf("unexpected distribution: %+v", r)
	}
	for i := range obs {
		obs[i].Answer = "I choose 7 because it is good"
	}
	r = modelfingerprint.Evaluate("qwen3.7-max", obs, "")
	if r.Distribution.MeanJSD != nil || r.Distribution.Cells[0].Valid != 0 {
		t.Fatal("must not extract a number from off-format answer")
	}
	if modelfingerprint.JensenShannon(map[string]float64{"a": 1}, map[string]float64{"b": 1}) != 1 {
		t.Fatal("disjoint distributions")
	}
}

func TestPublicFingerprintRejectsCrossCellModelSwitches(t *testing.T) {
	for _, model := range []string{"qwen3.7-flash", "qwen3.7-max"} {
		t.Run(model, func(t *testing.T) {
			observations := []modelfingerprint.Observation{}
			for _, p := range modelfingerprint.Plan(model) {
				responseModel := "model-a"
				if strings.Contains(p.Cell, "chinese") || strings.HasSuffix(p.Cell, "_zh") {
					responseModel = "model-b"
				}
				tokens := int64(55)
				if p.Prompt == "a" {
					tokens = 7
				}
				observations = append(observations, modelfingerprint.Observation{ID: p.ID, Answer: "7", InputTokens: &tokens, Protocol: "chat_completions", ResponseModel: responseModel})
			}
			result := modelfingerprint.Evaluate(model, observations, "")
			if result.Tokenizer != nil && result.Tokenizer.Status != "inconclusive" {
				t.Fatal("cross-cell routing must not yield tokenizer comparison")
			}
			if result.Distribution != nil && result.Distribution.MeanJSD != nil {
				t.Fatal("cross-cell routing must not yield mean distance")
			}
		})
	}
}
