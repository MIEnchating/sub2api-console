package business

import "strings"

func NormalizePlatform(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	compact := strings.NewReplacer(" ", "", "-", "", "_", "").Replace(value)
	switch compact {
	case "sub2api", "newapi", "oneapi", "openai":
		return "openai"
	case "glm", "zhipu", "zhipuai":
		return "zhipu"
	case "claude", "anthropic":
		return "anthropic"
	case "google", "gemini":
		return "gemini"
	case "moonshot", "kimi":
		return "kimi"
	}
	return value
}
