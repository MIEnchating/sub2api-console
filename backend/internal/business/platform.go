package business

import "strings"

var compositeAccountPlatforms = map[string]struct{}{
	"anthropic":   {},
	"openai":      {},
	"gemini":      {},
	"antigravity": {},
	"grok":        {},
	"kimi":        {},
	"zhipu":       {},
	"deepseek":    {},
	"opencode":    {},
}

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

func AccountPlatformCanJoinGroup(accountPlatform, groupPlatform string) bool {
	accountPlatform = NormalizePlatform(accountPlatform)
	groupPlatform = NormalizePlatform(groupPlatform)
	if accountPlatform == "" || groupPlatform == "" {
		return false
	}
	if accountPlatform == groupPlatform {
		return true
	}
	if groupPlatform != "composite" {
		return false
	}
	_, supported := compositeAccountPlatforms[accountPlatform]
	return supported
}
