package upstreamerror

import "strings"

// IsCapacity recognizes temporary provider saturation, distinct from account quota.
func IsCapacity(message string) bool {
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
