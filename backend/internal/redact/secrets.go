package redact

import "regexp"

var secretPattern = regexp.MustCompile(`(?i)(authorization|access[_-]?token|refresh[_-]?token|client[_-]?secret|api[_-]?key|token|password|passwd|secret|cookie)\s*["']?\s*[:=]\s*["']?[^,;"'\s}]+|bearer\s+[A-Za-z0-9._~+/=-]+`)

func Secrets(value string) string {
	return secretPattern.ReplaceAllString(value, "$1=<已隐藏>")
}
