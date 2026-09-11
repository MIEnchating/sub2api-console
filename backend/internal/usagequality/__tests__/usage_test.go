package usagequality_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/usagequality"
)

func TestUsageRejectsMissingMalformedAndFractionalCounters(t *testing.T) {
	for _, value := range []any{nil, true, "", "NaN", -1, 0.5, math.Inf(1), math.NaN(), math.Exp2(63), json.Number("0.5")} {
		if _, valid := usagequality.Normalize(map[string]any{"input_tokens": value, "output_tokens": 0}); valid {
			t.Errorf("invalid input accepted: %v", value)
		}
	}
}

func TestUsageNormalizesExplicitZeroRepresentationsAndIgnoresUnrelatedFields(t *testing.T) {
	for _, value := range []any{0, int64(0), float64(0), "0", json.Number("0")} {
		counts, valid := usagequality.Normalize(map[string]any{"input_tokens": value, "output_tokens": value, "request_body": "private"})
		if !valid || len(counts) != 2 || !usagequality.Empty(counts) {
			t.Errorf("valid zero usage not normalized: %v, %v", counts, valid)
		}
	}
}

func TestUsageWithUnknownCacheOrImageCounterDoesNotConfirmEmpty(t *testing.T) {
	for _, field := range []string{"cache_read_tokens", "cache_creation_tokens", "image_count"} {
		if usagequality.Empty(map[string]any{"input_tokens": 0, "output_tokens": 0, field: nil}) {
			t.Errorf("unknown %s treated as zero", field)
		}
	}
}
