package usagequality

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Normalize requires both token counters. Missing usage must never become zero.
func Normalize(raw map[string]any) (map[string]any, bool) {
	result := make(map[string]any)
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "image_count"} {
		value, present := raw[key]
		if !present && key != "input_tokens" && key != "output_tokens" {
			continue
		}
		count, valid := counter(value)
		if !valid {
			return nil, false
		}
		result[key] = count
	}
	return result, true
}

func Empty(raw map[string]any) bool {
	counts, valid := Normalize(raw)
	if !valid {
		return false
	}
	for _, value := range counts {
		if value != int64(0) {
			return false
		}
	}
	return true
}

func counter(raw any) (int64, bool) {
	var text string
	switch value := raw.(type) {
	case int:
		return int64(value), value >= 0
	case int64:
		return value, value >= 0
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= math.Exp2(63) || math.Trunc(value) != value {
			return 0, false
		}
		return int64(value), true
	case json.Number:
		text = value.String()
	case string:
		text = strings.TrimSpace(value)
	default:
		return 0, false
	}
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil && value >= 0
}
