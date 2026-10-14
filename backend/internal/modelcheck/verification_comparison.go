package modelcheck

import (
	"encoding/json"
	"fmt"
)

// Compare only selected accounts in this task; no extra requests or credentials.
// A reference needs an official API endpoint and the exact requested model.
// OAuth has a different system prompt/transport and is deliberately excluded.
func addVerificationComparisons(results []map[string]any) {
	for _, target := range results {
		if target["checker"] != "verification" || target["source"] != "third_party" {
			continue
		}
		var evidence []any
		evidence, _ = target["evidence"].([]any)
		for _, reference := range results {
			if reference["source"] != "official_endpoint" || reference["claimed_model"] != target["claimed_model"] {
				continue
			}
			compared, correctnessDifferences, tokenCompared, tokenDifferences := compareVerificationProbes(target, reference)
			if compared == 0 {
				continue
			}
			detail := fmt.Sprintf("与本任务官方 API 账号 %v 的同模型、同题目、同协议结果比较：%d 项可比，%d 项正确性不同；%d 项有输入 Token，%d 项用量不同。网关提示词、分词器和采样都会影响结果；此对照不认证模型身份。", reference["account_id"], compared, correctnessDifferences, tokenCompared, tokenDifferences)
			evidence = append(evidence, map[string]any{"id": fmt.Sprintf("reference-%v", reference["account_id"]), "label": "官方来源对照", "status": "info", "detail": detail})
		}
		target["evidence"] = evidence
	}
}

func compareVerificationProbes(target, reference map[string]any) (compared, differences, tokenCompared, tokenDifferences int) {
	var left, right VerificationReport
	targetJSON, _ := json.Marshal(target)
	referenceJSON, _ := json.Marshal(reference)
	if json.Unmarshal(targetJSON, &left) != nil || json.Unmarshal(referenceJSON, &right) != nil {
		return
	}
	for _, a := range left.Probes {
		if a.Status == "error" || a.Protocol == "" {
			continue
		}
		for _, b := range right.Probes {
			if b.ID != a.ID || b.Round != a.Round || b.Prompt != a.Prompt || b.Protocol != a.Protocol || b.Status == "error" || b.ResponseModel != reference["claimed_model"] {
				continue
			}
			compared++
			if a.Status != b.Status {
				differences++
			}
			if a.Usage != nil && b.Usage != nil && a.Usage.InputTokens != nil && b.Usage.InputTokens != nil {
				tokenCompared++
				if *a.Usage.InputTokens != *b.Usage.InputTokens {
					tokenDifferences++
				}
			}
			break
		}
	}
	return
}
