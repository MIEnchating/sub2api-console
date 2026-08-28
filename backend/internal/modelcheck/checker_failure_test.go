package modelcheck

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingBundleSender struct {
	err error
}

func (sender failingBundleSender) Send(context.Context, string, string, string, int) (string, string, error) {
	return "", "", sender.err
}

type textBundleSender struct {
	text string
}

func (sender textBundleSender) Send(context.Context, string, string, string, int) (string, string, error) {
	return sender.text, "gpt-5.6-sol", nil
}

func TestSolCheckReportsRequestFailureWhenEveryBundleFails(t *testing.T) {
	profile, err := loadSolProfile()
	if err != nil {
		t.Fatal(err)
	}
	result, err := runSolCheck(context.Background(), failingBundleSender{err: visibleRequestError{message: "upstream unavailable"}}, profile, targetRequest{
		AccountID: "14", AccountName: "测试账号", Model: "gpt-5.6-sol", Rounds: 1, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["verdict"] != "ERROR" {
		t.Fatalf("verdict=%v want ERROR", result["verdict"])
	}
	if failure, _ := result["error"].(string); !strings.Contains(failure, "upstream unavailable") {
		t.Fatalf("error=%v", result["error"])
	}
}

func TestClaudeCheckReportsRequestFailureWhenEveryBundleFails(t *testing.T) {
	profiles, err := loadClaudeProfiles()
	if err != nil {
		t.Fatal(err)
	}
	result, err := runClaudeCheck(context.Background(), failingBundleSender{err: visibleRequestError{message: "account test rejected"}}, profiles, targetRequest{
		AccountID: "14", AccountName: "测试账号", Model: "claude-opus-5", Rounds: 1, TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["verdict"] != "ERROR" {
		t.Fatalf("verdict=%v want ERROR", result["verdict"])
	}
	if failure, _ := result["error"].(string); !strings.Contains(failure, "account test rejected") {
		t.Fatalf("error=%v", result["error"])
	}
}

func TestAccountTestErrorIsRedactedAndBounded(t *testing.T) {
	message := safeAccountTestError("api_key: top-secret " + strings.Repeat("x", 400))
	if strings.Contains(message, "top-secret") {
		t.Fatalf("secret was not redacted: %q", message)
	}
	if len([]rune(message)) > 303 {
		t.Fatalf("message was not truncated: %d runes", len([]rune(message)))
	}
}

func TestRequestBundleExplainsLegacyFixedGreeting(t *testing.T) {
	result := requestBundle(context.Background(), textBundleSender{text: "Hi! What would you like to work on?"}, targetRequest{
		AccountID: "16", AccountName: "测试账号", Model: "gpt-5.6-sol", TimeoutSeconds: 5,
	}, []probe{{ID: "probe-1", Kind: "numeric", Question: "1 + 1 = ?"}}, 1, "sol")

	if result.Err == nil || !strings.Contains(result.Err.Error(), "不支持行为检测") {
		t.Fatalf("error=%v", result.Err)
	}
}

func TestRequestBundleKeepsGenericInvalidJSONError(t *testing.T) {
	result := requestBundle(context.Background(), textBundleSender{text: "I cannot answer that request."}, targetRequest{
		AccountID: "16", AccountName: "测试账号", Model: "gpt-5.6-sol", TimeoutSeconds: 5,
	}, []probe{{ID: "probe-1", Kind: "numeric", Question: "1 + 1 = ?"}}, 1, "sol")

	if result.Err == nil || result.Err.Error() != "响应未包含符合要求的 JSON 数组" {
		t.Fatalf("error=%v", result.Err)
	}
}

func TestRequestFailureReasonPrioritizesLegacyVersionError(t *testing.T) {
	rows := []bundleResult{
		{Err: errors.New("响应未包含符合要求的 JSON 数组")},
		{Err: errors.New("目标 Sub2API 版本不支持行为检测：账号测试通道未转发检测题目，请升级 Sub2API 服务端")},
	}

	reason := requestFailureReason(rows)
	if reason != "目标 Sub2API 版本不支持行为检测：账号测试通道未转发检测题目，请升级 Sub2API 服务端" {
		t.Fatalf("reason=%q", reason)
	}
}
