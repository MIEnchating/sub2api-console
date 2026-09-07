package routing

import (
	"math"
	"math/big"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
)

func TestPriceFirstPrefersFreeAccountAtEqualHealthAndSpeed(t *testing.T) {
	free, paid := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	free.rate, paid.rate = big.NewRat(0, 1), big.NewRat(1, 10)
	free.strategy, paid.strategy = "price_first", "price_first"
	calculateGroupWeights([]*candidate{free, paid}, engineConfig{weightBudget: 400, priceExp: 1, speedExp: 1, performanceMinSamples: 5, speedAdvantageCap: 4, gateFloor: 40})
	if free.weight <= paid.weight {
		t.Fatalf("free account weight=%g is below paid 0.1 account weight=%g", free.weight, paid.weight)
	}
}

func TestPriceScoringPreservesOrderingBeyondFloatRange(t *testing.T) {
	for _, costs := range [][2]string{{"1e-1000", "1e-999"}, {"1e999", "1e1000"}} {
		t.Run(costs[0], func(t *testing.T) {
			cheap, paid := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
			var ok bool
			cheap.rate, ok = decimalutil.Parse(costs[0])
			if !ok {
				t.Fatalf("invalid fixture cost %s", costs[0])
			}
			paid.rate, ok = decimalutil.Parse(costs[1])
			if !ok {
				t.Fatalf("invalid fixture cost %s", costs[1])
			}
			cheap.strategy, paid.strategy = "price_first", "price_first"
			calculateGroupWeights([]*candidate{cheap, paid}, engineConfig{weightBudget: 400, priceExp: 1, speedExp: 1})
			if math.Abs(cheap.weight-312.5) > 1e-9 || math.Abs(paid.weight-87.5) > 1e-9 {
				t.Fatalf("equivalent 1:10 decimal costs must retain weights: cheap=%g paid=%g", cheap.weight, paid.weight)
			}
		})
	}
}

func TestWeakPriceExponentKeepsExtremelyDifferentPaidCostsComparable(t *testing.T) {
	cheap, paid := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	var ok bool
	cheap.rate, ok = decimalutil.Parse("1e-1000")
	if !ok {
		t.Fatal("invalid fixture cost")
	}
	paid.rate = big.NewRat(1, 1)
	cheap.strategy, paid.strategy = "price_first", "price_first"
	calculateGroupWeights([]*candidate{cheap, paid}, engineConfig{weightBudget: 400, priceExp: 0.000001, speedExp: 1})
	if !(cheap.weight > paid.weight && paid.weight > 190) {
		t.Fatalf("a weak exponent must preserve ordering without zeroing paid price score: cheap=%g paid=%g", cheap.weight, paid.weight)
	}
}

func TestAllFreeAccountsShareWeightBudget(t *testing.T) {
	first, second := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	first.rate, second.rate = big.NewRat(0, 1), big.NewRat(0, 1)
	first.strategy, second.strategy = "price_first", "price_first"
	calculateGroupWeights([]*candidate{first, second}, engineConfig{weightBudget: 400, priceExp: 100, speedExp: 100})
	if first.weight != 200 || second.weight != 200 || first.quality != 1 || second.quality != 1 {
		t.Fatalf("identical free resources must have full finite quality and equal weights: %#v %#v", first, second)
	}
}

func TestPermittedLargePriceExponentPreservesPriceOrdering(t *testing.T) {
	policy := routingPolicy()
	policy["weights"].(map[string]any)["price_exp"] = 100
	config, err := parseEngineConfig(policy)
	if err != nil {
		t.Fatal(err)
	}
	cheap, paid := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	cheap.rate, paid.rate = big.NewRat(1, 10000), big.NewRat(1, 100)
	cheap.strategy, paid.strategy = "price_first", "price_first"
	calculateGroupWeights([]*candidate{cheap, paid}, config)
	if cheap.weight <= paid.weight {
		t.Fatalf("price exponent overflow erased price ordering: cheap=%g paid=%g", cheap.weight, paid.weight)
	}
}
