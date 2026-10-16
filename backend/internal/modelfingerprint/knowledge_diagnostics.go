package modelfingerprint

import "fmt"

// Keep eligibility and its explanation in one place: a completed HTTP attempt
// is not usable evidence when the batch failed or its answers are incomplete.
func knowledgeBlockingReasons(result *KnowledgeResult, metadata []Observation) []string {
	reasons := []string{}
	if result.FailedRequests > 0 {
		reasons = append(reasons, fmt.Sprintf("%d 个请求失败", result.FailedRequests))
	}
	if result.MissingRequests > 0 {
		reasons = append(reasons, fmt.Sprintf("%d 个请求未返回", result.MissingRequests))
	}
	if result.TruncatedRequests > 0 {
		reasons = append(reasons, fmt.Sprintf("%d 个回答被截断", result.TruncatedRequests))
	}
	if result.Total == 0 {
		reasons = append(reasons, "公开参考没有可比较的知识题")
	} else if result.Valid*2 < result.Total {
		reasons = append(reasons, "有效答案覆盖不足，至少需要一半题目返回可解析且在范围内的数值")
	}
	if result.SelfTotal*2 < result.Total || result.SelfTotal <= 0 || result.SelfMismatches < 0 || result.SelfMismatches >= result.SelfTotal {
		reasons = append(reasons, "公开参考自测基线无效或覆盖不足")
	}
	protocol, model := "", ""
	missing, mixedProtocol, mixedModel := false, false, false
	for _, observation := range metadata {
		missing = missing || observation.Protocol == "" || observation.ResponseModel == ""
		if observation.Protocol != "" {
			mixedProtocol = mixedProtocol || (protocol != "" && protocol != observation.Protocol)
			protocol = observation.Protocol
		}
		if observation.ResponseModel != "" {
			mixedModel = mixedModel || (model != "" && model != observation.ResponseModel)
			model = observation.ResponseModel
		}
	}
	if missing {
		reasons = append(reasons, "部分知识题缺少返回协议或型号")
	}
	if mixedProtocol {
		reasons = append(reasons, "知识题返回协议不一致")
	}
	if mixedModel {
		reasons = append(reasons, "知识题返回型号不一致")
	}
	return reasons
}
