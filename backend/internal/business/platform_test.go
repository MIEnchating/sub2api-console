package business

import "testing"

func TestNormalizePlatformAliasesMatchOnboardingCatalogNames(t *testing.T) {
	tests := map[string]string{
		"OpenAI": "openai", "newapi": "openai", "GLM": "zhipu", "Zhipu AI": "zhipu",
		"Claude": "anthropic", "Google": "gemini", "Moonshot": "kimi",
	}
	for input, expected := range tests {
		if actual := NormalizePlatform(input); actual != expected {
			t.Fatalf("NormalizePlatform(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestAccountPlatformCanJoinCompositeGroup(t *testing.T) {
	platforms := []string{"anthropic", "openai", "gemini", "antigravity", "grok", "kimi", "zhipu", "deepseek", "opencode"}
	for _, platform := range platforms {
		if !AccountPlatformCanJoinGroup(platform, platform) {
			t.Fatalf("%s account should join its own platform group", platform)
		}
		if !AccountPlatformCanJoinGroup(platform, "composite") {
			t.Fatalf("%s account should join a composite group", platform)
		}
	}
	if AccountPlatformCanJoinGroup("openai", "anthropic") {
		t.Fatal("openai account should not join an anthropic group")
	}
}
