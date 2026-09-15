package officialpricing_test

import (
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestMalformedOfficialTokenBoundariesReturnValidationErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture string
		from    string
		to      string
		parse   func([]byte) ([]pricing.Price, error)
	}{
		{"glm", "glm.md", "32K", "1..5K", pricing.ParseGLM},
		{"minimax", "minimax.md", "512k", "1..5k", pricing.ParseMiniMax},
		{"qwen", "qwen.html", "256K", "1..5K", pricing.ParseQwen},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := strings.ReplaceAll(string(fixture(t, test.fixture)), test.from, test.to)
			if _, err := test.parse([]byte(raw)); err == nil {
				t.Fatal("malformed token boundary accepted")
			}
		})
	}
}

func TestFractionalTokenBoundaryDoesNotRoundIntoBillingCondition(t *testing.T) {
	raw := strings.ReplaceAll(string(fixture(t, "minimax.md")), "512k", "0.0005k")
	if _, err := pricing.ParseMiniMax([]byte(raw)); err == nil {
		t.Fatal("fractional token boundary rounded into billing condition")
	}
}
