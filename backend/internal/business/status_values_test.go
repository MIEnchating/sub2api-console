package business

import (
	"testing"
)

func TestCanonicalUpstreamAuthStatuses(t *testing.T) {
	statuses := []string{
		UpstreamAuthStatusAuthenticated,
		UpstreamAuthStatusRecovered,
		UpstreamAuthStatusPendingVerification,
		UpstreamAuthStatusUnconfirmed,
		UpstreamAuthStatusRecoveryTemporarilyFailed,
		UpstreamAuthStatusInvalid,
		UpstreamAuthStatusConfigurationError,
	}
	want := []string{"已鉴权", "已恢复", "待验证", "未确认", "恢复暂时失败", "鉴权失效", "配置错误"}
	if len(statuses) != len(want) {
		t.Fatalf("auth status count = %d, want %d", len(statuses), len(want))
	}
	for index := range want {
		if statuses[index] != want[index] {
			t.Fatalf("auth status %d = %q, want %q", index, statuses[index], want[index])
		}
	}
}

func TestUnknownUpstreamAuthStatusIsNotAuthenticated(t *testing.T) {
	if !UpstreamAuthStatusIsReady("已发现鉴权记录") {
		t.Fatal("legacy ready status must remain compatible")
	}
	if UpstreamAuthStatusIsReady("上游新增状态") {
		t.Fatal("unknown upstream auth status must not be treated as authenticated")
	}
}
