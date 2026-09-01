package business

import "strings"

const (
	UpstreamAuthStatusAuthenticated             = "已鉴权"
	UpstreamAuthStatusRecovered                 = "已恢复"
	UpstreamAuthStatusPendingVerification       = "待验证"
	UpstreamAuthStatusUnconfirmed               = "未确认"
	UpstreamAuthStatusRecoveryTemporarilyFailed = "恢复暂时失败"
	UpstreamAuthStatusInvalid                   = "鉴权失效"
	UpstreamAuthStatusConfigurationError        = "配置错误"
)

var readyUpstreamAuthStatuses = map[string]struct{}{
	strings.ToLower(UpstreamAuthStatusAuthenticated): {},
	strings.ToLower(UpstreamAuthStatusRecovered):     {},
	strings.ToLower("已发现鉴权记录"):                       {},
	strings.ToLower("已认证"):                           {},
	"authenticated":                                  {},
	"authorized":                                     {},
	"healthy":                                        {},
	"valid":                                          {},
	"ok":                                             {},
	"succeeded":                                      {},
}

func UpstreamAuthStatusIsReady(value string) bool {
	_, ready := readyUpstreamAuthStatuses[strings.ToLower(strings.TrimSpace(value))]
	return ready
}
