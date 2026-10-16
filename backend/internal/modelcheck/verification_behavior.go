package modelcheck

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

const behaviorFingerprintVersion = "short-answer-distribution-v1"
const behaviorSamplesPerCell = 25
const behaviorRequestCount = 8 * behaviorSamplesPerCell

type behaviorCell struct {
	id, label, prompt string
	choices           []string
}

func behaviorCells() []behaviorCell {
	return []behaviorCell{
		{"number_en", "数字 · 英文", "Choose one digit from 0 to 9. Reply with only the digit.", []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
		{"number_zh", "数字 · 中文", "从 0 到 9 中选择一个数字。只回复这个数字。", []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}},
		{"color_en", "颜色 · 英文", "Choose one color: red, blue, green, yellow. Reply with only its English name.", []string{"red", "blue", "green", "yellow"}},
		{"color_zh", "颜色 · 中文", "从红、蓝、绿、黄中选择一种颜色。只回复一个汉字。", []string{"红", "蓝", "绿", "黄"}},
		{"coin_en", "硬币 · 英文", "Choose heads or tails. Reply with only that word.", []string{"heads", "tails"}},
		{"coin_zh", "硬币 · 中文", "从正面和反面中选择一个。只回复正面或反面。", []string{"正面", "反面"}},
		{"direction_en", "方向 · 英文", "Choose one direction: north, south, east, west. Reply with only that word.", []string{"north", "south", "east", "west"}},
		{"direction_zh", "方向 · 中文", "从东、南、西、北中选择一个方向。只回复一个汉字。", []string{"东", "南", "西", "北"}},
	}
}

// Interleave cells so each cell spans the sampling session rather than one time slice.
func verificationBehaviorChallenges() []verificationChallenge {
	result := make([]verificationChallenge, 0, behaviorRequestCount)
	for i := 0; i < behaviorSamplesPerCell; i++ {
		for _, cell := range behaviorCells() {
			result = append(result, verificationChallenge{id: fmt.Sprintf("behavior_%s_%02d", cell.id, i+1), label: fmt.Sprintf("%s · %d/%d", cell.label, i+1, behaviorSamplesPerCell), prompt: cell.prompt, expected: "记录选项分布，不按选项对错评分", measurement: true, behavior: true})
		}
	}
	return result
}

type BehaviorFingerprintCell struct {
	ID        string         `json:"id"`
	Label     string         `json:"label"`
	Counts    map[string]int `json:"counts"`
	Valid     int            `json:"valid"`
	Invalid   int            `json:"invalid"`
	Errors    int            `json:"errors"`
	Missing   int            `json:"missing"`
	Status    string         `json:"status"`
	SelfJSD   *float64       `json:"self_jsd,omitempty"`
	Protocols []string       `json:"protocols"`
	Models    []string       `json:"models"`
}

type BehaviorSamplingParameters struct {
	Stream          bool   `json:"stream"`
	Temperature     string `json:"temperature"`
	ReasoningEffort string `json:"reasoning_effort"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	Instructions    string `json:"instructions,omitempty"`
}

type BehaviorFingerprint struct {
	Version    string                     `json:"version"`
	Status     string                     `json:"status"`
	Summary    string                     `json:"summary"`
	Identity   string                     `json:"identity"`
	Profile    string                     `json:"profile"`
	Parameters BehaviorSamplingParameters `json:"parameters"`
	Sampling   string                     `json:"sampling"`
	Planned    int                        `json:"planned"`
	Valid      int                        `json:"valid"`
	Cells      []BehaviorFingerprintCell  `json:"cells"`
}

func verificationBehaviorReport(probes []VerificationProbe, enabled bool) *BehaviorFingerprint {
	if !enabled {
		return nil
	}
	result := &BehaviorFingerprint{
		Version: behaviorFingerprintVersion, Status: "collected", Identity: "inconclusive", Profile: "verification-medium-v1", Planned: behaviorRequestCount,
		Sampling: "沿用当前检测适配器：流式、请求 medium 思考等级（Gemini 使用默认）、温度未指定。上游默认参数与隐藏提示词不可验证，不跨配置比较。",
		Summary:  "已采集回答分布；奇偶样本 JSD 仅描述本次差异，未经身份阈值校准，无法判定模型身份。",
	}
	byID := map[string]VerificationProbe{}
	for _, probe := range probes {
		byID[probe.ID] = probe
	}
	protocols, models := []string{}, []string{}
	for _, cell := range behaviorCells() {
		sample := behaviorSample(cell, byID)
		for _, protocol := range sample.Protocols {
			if !slices.Contains(protocols, protocol) {
				protocols = append(protocols, protocol)
			}
		}
		for _, model := range sample.Models {
			if !slices.Contains(models, model) {
				models = append(models, model)
			}
		}
		result.Cells = append(result.Cells, sample)
		result.Valid += sample.Valid
		if sample.Status == "incomparable" {
			result.Status = "incomparable"
		} else if sample.Status != "collected" && result.Status != "incomparable" {
			result.Status = "incomplete"
		}
	}
	if len(protocols) > 1 || len(models) > 1 {
		result.Status = "incomparable"
	}
	if result.Status == "incomplete" {
		result.Summary = "存在缺失、无效或失败样本；不足 10 个有效样本或奇偶任侧不足 5 个时不计算 JSD。已保留可用分布，无法判定模型身份。"
	}
	if result.Status == "incomparable" {
		result.Summary = "采样的协议或返回型号缺失／变化，无法形成可比指纹；分布仅供查看，无法判定模型身份。"
	}
	return result
}

func behaviorSample(cell behaviorCell, byID map[string]VerificationProbe) BehaviorFingerprintCell {
	result := BehaviorFingerprintCell{ID: cell.id, Label: cell.label, Counts: map[string]int{}, Status: "collected", Protocols: []string{}, Models: []string{}}
	halves := [2]map[string]int{{}, {}}
	totals := [2]int{}
	metadataMissing := false
	for i := 0; i < behaviorSamplesPerCell; i++ {
		probe, exists := byID[fmt.Sprintf("behavior_%s_%02d", cell.id, i+1)]
		if !exists {
			result.Missing++
			continue
		}
		if probe.Status == "error" {
			result.Errors++
			continue
		}
		answer := strings.ToLower(strings.TrimSpace(probe.Answer))
		if probe.Status != "observed" || probe.AnswerTruncated || !slices.Contains(cell.choices, answer) {
			result.Invalid++
			continue
		}
		result.Valid++
		result.Counts[answer]++
		halves[i%2][answer]++
		totals[i%2]++
		metadataMissing = metadataMissing || probe.Protocol == "" || probe.ResponseModel == ""
		if probe.Protocol != "" && !slices.Contains(result.Protocols, probe.Protocol) {
			result.Protocols = append(result.Protocols, probe.Protocol)
		}
		if probe.ResponseModel != "" && !slices.Contains(result.Models, probe.ResponseModel) {
			result.Models = append(result.Models, probe.ResponseModel)
		}
	}
	if result.Valid != behaviorSamplesPerCell {
		result.Status = "incomplete"
	}
	if result.Valid > 0 && (metadataMissing || len(result.Models) != 1 || len(result.Protocols) != 1) {
		result.Status = "incomparable"
	}
	if result.Status != "incomparable" && result.Valid >= 10 && totals[0] >= 5 && totals[1] >= 5 {
		jsd := 0.0
		for _, choice := range cell.choices {
			p, q := float64(halves[0][choice])/float64(totals[0]), float64(halves[1][choice])/float64(totals[1])
			mean := (p + q) / 2
			if p > 0 {
				jsd += p * math.Log2(p/mean) / 2
			}
			if q > 0 {
				jsd += q * math.Log2(q/mean) / 2
			}
		}
		result.SelfJSD = &jsd
	}
	return result
}
