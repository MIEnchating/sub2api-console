package configstore

// Built-in values are protocol constants. Only their display order is configurable.
var builtInDictionaryValues = map[string][]DictionaryEntry{
	"account_type": {
		{Value: "apikey", Name: "API Key"}, {Value: "oauth", Name: "OAuth"},
		{Value: "sub2api", Name: "Sub2API"}, {Value: "newapi", Name: "New API"}, {Value: "oneapi", Name: "OneAPI"},
	},
	"upstream_type": {
		{Value: "sub2api", Name: "Sub2API"}, {Value: "newapi", Name: "New API"}, {Value: "oneapi", Name: "OneAPI"},
		{Value: "custom", Name: "自定义上游"}, {Value: "apikey", Name: "API Key"},
	},
	"auth_status": {
		{Value: "已鉴权", Name: "已鉴权"}, {Value: "已恢复", Name: "已恢复"}, {Value: "待验证", Name: "待验证"},
		{Value: "未确认", Name: "未确认"}, {Value: "恢复暂时失败", Name: "恢复暂时失败"},
		{Value: "鉴权失效", Name: "鉴权失效"}, {Value: "配置错误", Name: "配置错误"},
	},
	"scheduling_strategy": {
		{Value: "balanced", Name: "均衡"}, {Value: "price_first", Name: "价格优先"},
		{Value: "speed_first", Name: "速度优先"}, {Value: "reliability", Name: "稳定优先"},
	},
	"task_status": {
		{Value: "queued", Name: "排队中"}, {Value: "running", Name: "进行中"}, {Value: "waiting_input", Name: "等待输入"},
		{Value: "succeeded", Name: "已成功"}, {Value: "partial", Name: "部分完成"},
		{Value: "failed", Name: "已失败"}, {Value: "cancelled", Name: "已取消"},
	},
	"account_status": {
		{Value: "manual_priority", Name: "手动控制"}, {Value: "healthy", Name: "健康"},
		{Value: "degraded", Name: "降级"}, {Value: "cost_blocked", Name: "成本墙拦截"},
		{Value: "concurrency_limited", Name: "等待并发额度"},
		{Value: "fused", Name: "已熔断"}, {Value: "survivor", Name: "保底强留"},
		{Value: "paused", Name: "已暂停"}, {Value: "disabled", Name: "已停用"},
		{Value: "excluded", Name: "已排除"}, {Value: "unknown", Name: "待探测"},
	},
	"alert_status": {
		{Value: "firing", Name: "告警中"}, {Value: "recovered", Name: "已恢复"},
		{Value: "suppressed", Name: "规则已停用"}, {Value: "closed", Name: "已关闭"},
	},
	"kuma_monitor_type": {
		{Value: "http", Name: "HTTP(S)"}, {Value: "keyword", Name: "HTTP(S) 关键字"},
		{Value: "port", Name: "TCP 端口"}, {Value: "ping", Name: "Ping"},
		{Value: "dns", Name: "DNS"}, {Value: "push", Name: "Push 推送"}, {Value: "group", Name: "分组"},
	},
}
