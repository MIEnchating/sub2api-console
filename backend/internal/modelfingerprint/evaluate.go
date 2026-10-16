package modelfingerprint

import (
	"fmt"
	"strings"
)

type Observation struct {
	ID            string
	Answer        string
	InputTokens   *int64
	Protocol      string
	ResponseModel string
	Error         bool
	Truncated     bool
}
type Result struct {
	Version      string              `json:"version"`
	Digest       string              `json:"digest"`
	Model        string              `json:"model"`
	Status       string              `json:"status"`
	Summary      string              `json:"summary"`
	Planned      int                 `json:"planned"`
	Sources      []Source            `json:"sources"`
	Knowledge    *KnowledgeResult    `json:"knowledge,omitempty"`
	Tokenizer    *TokenizerResult    `json:"tokenizer,omitempty"`
	Distribution *DistributionResult `json:"distribution,omitempty"`
}

func Evaluate(model string, observations []Observation, incompatibility string) Result {
	result := Result{Version: Version, Digest: Digest(), Model: model, Status: "inconclusive", Sources: []Source{}, Summary: "该精确型号没有可用公开参考；未套用其他版本。"}
	p := lookup(model)
	if p == nil {
		return result
	}
	result.Model = p.Model
	result.Sources = append([]Source(nil), p.Sources...)
	plan := Plan(model)
	result.Planned = len(plan)
	if p.Unavailable != "" {
		result.Summary = p.Unavailable
		return result
	}
	if incompatibility != "" {
		result.Planned = 0
		result.Summary = incompatibility
		return result
	}
	byID := map[string]Observation{}
	for _, o := range observations {
		byID[o.ID] = o
	}
	if p.KBF != nil {
		result.Knowledge = evaluateKnowledge(p.KBF, plan, byID)
	}
	if p.Tokenizer != nil {
		result.Tokenizer = evaluateTokenizer(p.Tokenizer, byID)
	}
	if p.Distribution != nil {
		result.Distribution = evaluateDistribution(p.Distribution, byID)
	}
	result.Summary = "仅有分词或回答分布证据，缺少该型号的身份判别校准；请查看差异，不能据此认定真伪。"
	if k := result.Knowledge; k != nil {
		result.Status = k.Status
		switch k.Status {
		case "mismatch":
			result.Summary = fmt.Sprintf("知识边界与公开参考显著不符（%d/%d 项差异），疑似模型替换；也可能受版本、提供商、量化或隐藏提示词影响，建议复测核对。", k.Mismatches, k.Valid)
		case "matched":
			result.Summary = "知识边界未检出超出公开自测基线的差异；与该参考一致，不等同于官方身份认证。"
		default:
			result.Summary = fmt.Sprintf("知识边界无法判定：%s；已保留有效答案 %d/%d 题。请查看失败项或返回原文，核对接口后重试。", strings.Join(k.Reasons, "；"), k.Valid, k.Total)
		}
		if result.Status == "matched" && result.Tokenizer != nil && result.Tokenizer.Status != "consistent" {
			result.Status = "inconclusive"
			result.Summary = "知识边界未检出显著差异，但分词证据缺失、不稳定或冲突，暂不作一致结论。"
		}
	}
	return result
}

func consistentMetadata(observations []Observation) bool {
	protocol, model := "", ""
	for _, o := range observations {
		if o.Error || o.Protocol == "" || o.ResponseModel == "" {
			return false
		}
		if protocol != "" && (protocol != o.Protocol || model != o.ResponseModel) {
			return false
		}
		protocol, model = o.Protocol, o.ResponseModel
	}
	return len(observations) > 0
}
