package modelfingerprint_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
)

func TestKnowledgeFailureExplainsActualBlockingBatch(t *testing.T) {
	observations := answerReference("claude-opus-5", false)
	// The public plan exposes the reference ID without the model-check prefix.
	found := false
	for i := range observations {
		if strings.Contains(observations[i].ID, "crypto_params_0") {
			observations[i].Error = true
			found = true
		}
	}
	if !found {
		t.Fatal("fixture requires the failed crypto batch")
	}
	result := modelfingerprint.Evaluate("claude-opus-5", observations, "")
	if result.Knowledge.Status != "inconclusive" || result.Knowledge.Valid != 404 {
		t.Fatalf("unexpected coverage: %+v", result.Knowledge)
	}
	if !strings.Contains(result.Summary, "1 个请求失败") || !strings.Contains(result.Summary, "404/414") {
		t.Fatalf("missing actual failure and retained coverage: %s", result.Summary)
	}
	if strings.Contains(result.Summary, "协议／型号变化") {
		t.Fatal("must not invent metadata changes")
	}
}

func TestKnowledgeDiagnosticsSeparateMissingTruncatedParsingAndMetadata(t *testing.T) {
	for _, scenario := range []struct{ name, want string }{
		{"missing", "1 个请求未返回"}, {"truncated", "1 个回答被截断"},
		{"parsing", "有效答案覆盖不足"}, {"model", "返回型号不一致"}, {"protocol", "返回协议不一致"},
		{"metadata", "缺少返回协议或型号"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			observations := answerReference("gpt-6-astra", false)
			switch scenario.name {
			case "missing":
				observations = observations[1:]
			case "truncated":
				observations[0].Truncated = true
			case "parsing":
				for i := range observations {
					observations[i].Answer = "cannot answer"
				}
			case "model":
				observations[0].ResponseModel = "different-model"
			case "protocol":
				observations[0].Protocol = "anthropic_messages"
			case "metadata":
				observations[0].ResponseModel = ""
			}
			result := modelfingerprint.Evaluate("gpt-6-astra", observations, "")
			if result.Status != "inconclusive" || !strings.Contains(result.Summary, scenario.want) {
				t.Fatalf("missing specific diagnostic %q: %+v", scenario.want, result)
			}
		})
	}
}
