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
