package upstreamerror

import (
	"encoding/json"
	"io"
	"strings"
)

// IsCapacity recognizes temporary provider saturation, distinct from account quota.
func IsCapacity(message string) bool {
	trimmed := strings.TrimSpace(message)
	if len(trimmed) > 0 && strings.ContainsRune("{[\"", rune(trimmed[0])) {
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.UseNumber()
		var value, trailing any
		if decoder.Decode(&value) == nil && decoder.Decode(&trailing) == io.EOF {
			return capacityErrorValue(value, 0)
		}
	}
	// Keep plain and truncated error excerpts compatible with text matching.
	return capacityText(message)
}

func capacityErrorValue(value any, depth int) bool {
	if depth > 16 {
		return false
	}
	switch value := value.(type) {
	case string:
		return capacityText(value)
	case map[string]any:
		// Do not classify request IDs, model names or arbitrary metadata as errors.
		for _, key := range []string{"message", "code", "type", "error", "response"} {
			if capacityErrorValue(value[key], depth+1) {
				return true
			}
		}
	}
	return false
}

func capacityText(message string) bool {
	message = strings.ReplaceAll(strings.ReplaceAll(message, "_", " "), "-", " ")
	text := strings.ToLower(strings.Join(strings.Fields(message), " "))
	for _, marker := range []string{
		"overloaded", "service is busy", "server is busy", "model is at capacity",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
