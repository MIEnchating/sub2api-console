package writebackpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

const DefaultConcurrency = 4

// Verification reports whether remote writes require a separate readback.
// Missing configuration deliberately defaults to false.
func Verification(document map[string]any) (bool, error) {
	if document == nil {
		return false, nil
	}
	raw, present := document["writeback"]
	if !present {
		return false, nil
	}
	writeback, ok := raw.(map[string]any)
	if !ok {
		return false, errors.New("策略字段 writeback 必须是对象")
	}
	raw, present = writeback["verification"]
	if !present {
		return false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, errors.New("策略字段 writeback.verification 必须是布尔值")
	}
	return value, nil
}

func Concurrency(document map[string]any) (int, error) {
	if document == nil {
		return DefaultConcurrency, nil
	}
	raw, present := document["writeback"]
	if !present {
		return DefaultConcurrency, nil
	}
	writeback, ok := raw.(map[string]any)
	if !ok {
		return 0, errors.New("策略字段 writeback 必须是对象")
	}
	raw, present = writeback["concurrency"]
	if !present {
		return DefaultConcurrency, nil
	}
	value, err := policyInteger(raw)
	if err != nil || value < 1 || value > 16 {
		return 0, errors.New("策略字段 writeback.concurrency 必须在 1 到 16 之间")
	}
	return int(value), nil
}

func policyInteger(raw any) (int64, error) {
	switch value := raw.(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case json.Number:
		return value.Int64()
	case float64:
		if value != float64(int64(value)) {
			return 0, errors.New("not integer")
		}
		return int64(value), nil
	case string:
		return strconv.ParseInt(value, 10, 64)
	default:
		return 0, fmt.Errorf("not integer: %T", raw)
	}
}
