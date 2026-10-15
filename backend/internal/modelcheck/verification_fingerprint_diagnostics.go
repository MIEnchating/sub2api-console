package modelcheck

import (
	"fmt"
	"strings"
)

func fingerprintSample(challenge verificationChallenge, first, repeat VerificationProbe, foundFirst, foundRepeat bool) VerificationFingerprintSample {
	sample := VerificationFingerprintSample{
		ID: strings.TrimPrefix(challenge.id, "fingerprint_"), Label: strings.TrimSuffix(challenge.label, "采样"), Status: "stable",
		Protocol: first.Protocol, RepeatProtocol: repeat.Protocol, Model: first.ResponseModel, RepeatModel: repeat.ResponseModel,
	}
	if first.Usage != nil {
		sample.First = first.Usage.InputTokens
	}
	if repeat.Usage != nil {
		sample.Repeat = repeat.Usage.InputTokens
	}
	details := []string{}
	incomplete, missing, changed := false, false, false
	for _, check := range []struct {
		label  string
		probe  VerificationProbe
		found  bool
		tokens *int64
	}{
		{"首次", first, foundFirst, sample.First}, {"复测", repeat, foundRepeat, sample.Repeat},
	} {
		if !check.found {
			incomplete = true
			details = append(details, check.label+"未执行")
			continue
		}
		if check.probe.Status == "error" {
			incomplete = true
			details = append(details, check.label+"请求失败，请查看逐项请求错误")
			continue
		}
		if check.tokens == nil {
			missing = true
			details = append(details, check.label+"未返回有效输入用量")
		}
		if check.probe.ResponseModel == "" {
			missing = true
			details = append(details, check.label+"未返回型号")
		}
		if check.probe.Protocol == "" || check.probe.Protocol == "other" {
			missing = true
			details = append(details, check.label+"协议未识别")
		}
	}
	if sample.First != nil && sample.Repeat != nil && *sample.First != *sample.Repeat {
		changed = true
		details = append(details, fmt.Sprintf("输入用量变化：%d → %d Token", *sample.First, *sample.Repeat))
	}
	if first.ResponseModel != "" && repeat.ResponseModel != "" && first.ResponseModel != repeat.ResponseModel {
		changed = true
		details = append(details, "首次与复测返回型号不同")
	}
	if first.Protocol != "" && repeat.Protocol != "" && first.Protocol != "other" && repeat.Protocol != "other" && first.Protocol != repeat.Protocol {
		changed = true
		details = append(details, "首次与复测协议不同")
	}
	switch {
	case changed:
		sample.Status = "unstable"
	case incomplete:
		sample.Status = "incomplete"
	case missing:
		sample.Status = "unavailable"
	}
	sample.Detail = strings.Join(details, "；")
	return sample
}

func fingerprintSampleDelta(sample *VerificationFingerprintSample, baseline VerificationFingerprintSample) {
	if sample.Status != "stable" {
		return
	}
	if baseline.Status != "stable" {
		sample.Status = "unavailable"
		sample.Detail = "本项重复采样一致，但基准文本不可用，无法计算差分。基准原因：" + baseline.Detail
		return
	}
	if sample.Protocol != baseline.Protocol || sample.Model != baseline.Model {
		sample.Status = "unstable"
		sample.Detail = "本项重复采样一致，但返回型号或协议与基准文本不同，无法计算可比差分。"
		return
	}
	delta := *sample.First - *baseline.First
	sample.Delta = &delta
	sample.Detail = "首次与复测的用量、型号和协议一致，已减去基准文本用量。"
}

func fingerprintStatusPriority(status string) int {
	switch status {
	case "unstable":
		return 3
	case "incomplete":
		return 2
	case "unavailable":
		return 1
	default:
		return 0
	}
}
