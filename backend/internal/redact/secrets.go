package redact

import "regexp"

var secretPattern = regexp.MustCompile(`(?i)(authorization|access[_-]?token|refresh[_-]?token|client[_-]?secret|admin[_-]?key|api[_-]?key|token|password|passwd|secret|cookie)\s*["']?\s*[:=]\s*(?:"(?:\\.|[^"\\])*(?:"|$)|'(?:\\.|[^'\\])*(?:'|$)|(?:bearer|basic)\s+[^,;"'\s}]+|["']?[^,;"'\s}]+)|bearer\s+[A-Za-z0-9._~+/=-]+`)

func Secrets(value string) string {
	return secretPattern.ReplaceAllString(value, "$1=<已隐藏>")
}
