package modelcheck

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
)

const precheckMode = "precheck"
const combinedMode = "both"

type PrecheckQuestionResult struct {
	ID        string `json:"id"`
	Verdict   string `json:"verdict"`
	Answer    string `json:"answer,omitempty"`
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id"`
}

type PrecheckResult struct {
	Verdict        string                   `json:"verdict"`
	ProfileVersion string                   `json:"profile_version"`
	Questions      []PrecheckQuestionResult `json:"questions"`
}

func validAnimationMode(mode string) bool {
	return mode == "" || mode == "animation" || mode == precheckMode || mode == combinedMode
}

func normalizePrecheckQuestions(mode string, questions []string) ([]string, error) {
	if mode != precheckMode && mode != combinedMode {
		if questions != nil {
			return nil, errors.New("只有前置检测可以选择检测题目")
		}
		return nil, nil
	}
	if questions == nil {
		return []string{"candy", "knowledge-cutoff"}, nil
	}
	if len(questions) < 1 || len(questions) > 2 {
		return nil, errors.New("请选择至少一道前置检测题目")
	}
	seen := map[string]bool{}
	for _, id := range questions {
		if (id != "candy" && id != "knowledge-cutoff") || seen[id] {
			return nil, errors.New("前置检测题目无效或重复")
		}
		seen[id] = true
	}
	result := make([]string, 0, len(questions))
	for _, id := range []string{"candy", "knowledge-cutoff"} {
		if seen[id] {
			result = append(result, id)
		}
	}
	return result, nil
}

func animationModeLabel(mode string) string {
	if mode == combinedMode {
		return "前置与动画检测"
	}
	if mode == precheckMode {
		return "前置检测"
	}
	return "动画检测"
}

func runPrecheckTarget(ctx context.Context, client *http.Client, credential directCredential, timeout int, questions []string, result *AnimationResult) error {
	profile := builtinAstraProfile()
	check := &PrecheckResult{Verdict: "error", ProfileVersion: profile.Version, Questions: make([]PrecheckQuestionResult, 0, 2)}
	result.Precheck = check
	for _, question := range profile.Questions {
		if !slices.Contains(questions, question.ID) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		row := PrecheckQuestionResult{ID: question.ID, Verdict: "error", RequestID: result.RequestID + "-" + question.ID}
		sender := directBundleSender{client: client, credential: credential, requestID: row.RequestID}
		effort := ""
		if result.Model == astraModel {
			effort = "low"
		}
		text, model, err := sender.SendWithReasoning(ctx, result.AccountID, result.Model, question.Question, timeout, effort)
		if err == nil && credential.Secret != "" && strings.Contains(text, credential.Secret) {
			err = errors.New("上游回答包含敏感信息，已拒绝展示")
		}
		if err != nil {
			row.Error = safeCredentialError(err)
		} else {
			row.Answer = safeCredentialText(text)
			verdict, _ := classifyAstraAnswer(question.ID, text)
			switch verdict {
			case "MATCH":
				row.Verdict = "passed"
			case "MISMATCH":
				row.Verdict = "not_passed"
			default:
				row.Verdict = "inconclusive"
			}
			if model != "" {
				result.ResponseModel = model
			}
		}
		check.Questions = append(check.Questions, row)
	}
	check.Verdict = "passed"
	for _, question := range check.Questions {
		if question.Verdict == "error" {
			check.Verdict = "error"
			return errors.New("前置检测请求失败，请查看题目结果后重试")
		}
		if question.Verdict == "not_passed" {
			check.Verdict = "not_passed"
		}
		if question.Verdict == "inconclusive" && check.Verdict == "passed" {
			check.Verdict = "inconclusive"
		}
	}
	return ctx.Err()
}
