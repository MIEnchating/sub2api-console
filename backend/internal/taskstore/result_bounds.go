package taskstore

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type truncationStat struct {
	Path     string `json:"path"`
	Total    int    `json:"total"`
	Retained int    `json:"retained"`
}

func boundedTaskResult(result map[string]any, maximum int) map[string]any {
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) <= maximum {
		return result
	}
	// Decode once. Each attempt copies only retained entries so the returned
	// result does not keep discarded array elements alive through its capacity.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var source map[string]any
	if err := decoder.Decode(&source); err != nil {
		return result
	}
	for capPerArray := 1000; capPerArray >= 0; capPerArray /= 2 {
		stats := make([]truncationStat, 0)
		candidate := truncateTaskArrays(source, "$", capPerArray, &stats).(map[string]any)
		total, retained := 0, 0
		for _, stat := range stats {
			total += stat.Total
			retained += stat.Retained
		}
		candidate["truncated"] = true
		candidate["total"] = total
		candidate["retained"] = retained
		candidate["truncation"] = stats
		if bounded, marshalErr := json.Marshal(candidate); marshalErr == nil && len(bounded) <= maximum {
			return candidate
		}
		if capPerArray == 0 {
			break
		}
	}
	fallback := map[string]any{
		"truncated": true, "total": 0, "retained": 0,
		"truncation": []truncationStat{}, "detail": "任务结果超过持久化上限，明细未保留",
	}
	for _, key := range []string{"error", "summary", "host", "account_id", "request_id", "run_key"} {
		if text, ok := result[key].(string); ok && len(text) <= 4096 {
			fallback[key] = text
		}
	}
	return fallback
}

func truncateTaskArrays(value any, path string, capPerArray int, stats *[]truncationStat) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[key] = truncateTaskArrays(child, path+"."+key, capPerArray, stats)
		}
		return result
	case []any:
		retained := min(len(item), capPerArray)
		if retained < len(item) {
			*stats = append(*stats, truncationStat{Path: path, Total: len(item), Retained: retained})
		}
		result := make([]any, retained)
		for index := range result {
			result[index] = truncateTaskArrays(item[index], fmt.Sprintf("%s[%d]", path, index), capPerArray, stats)
		}
		return result
	default:
		return value
	}
}
