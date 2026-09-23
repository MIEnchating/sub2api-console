package modelcheck

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type TerminalContinuityRound struct {
	Round         int    `json:"round"`
	Verdict       string `json:"verdict"`
	RequestID     string `json:"request_id"`
	Response      string `json:"response,omitempty"`
	ResponseModel string `json:"response_model,omitempty"`
	Error         string `json:"error,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	CompletedAt   string `json:"completed_at"`
}

func validateTerminalRounds(rounds int) error {
	if rounds < 0 || rounds > 20 {
		return errors.New("终端检测轮数必须在 1 到 20 之间")
	}
	return nil
}

func (s *Service) runTerminalRounds(ctx context.Context, account selectedAccount, timeout int, model, requestID string, rounds int) TerminalContinuityResult {
	result := TerminalContinuityResult{AccountID: account.ID, AccountName: account.Name, Model: model, RequestID: requestID, Verdict: "normal"}
	priority := map[string]int{"normal": 0, "inconclusive": 1, "suspected": 2, "error": 3}
	started := time.Now()
	for round := 1; round <= rounds; round++ {
		if ctx.Err() != nil {
			break
		}
		item := TerminalContinuityRound{Round: round, RequestID: fmt.Sprintf("%s-r%d", requestID, round), Verdict: "error"}
		begin := time.Now()
		response, responseModel, err := s.runTerminalContinuityTarget(ctx, account, timeout, model, item.RequestID)
		item.DurationMS = time.Since(begin).Milliseconds()
		item.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			item.Error = safeCredentialError(err)
		} else {
			item.Response, item.ResponseModel = safeCredentialText(response), safeCredentialText(responseModel)
			item.Verdict = classifyTerminalContinuity(response)
		}
		result.RoundResults = append(result.RoundResults, item)
		if len(result.RoundResults) == 1 || priority[item.Verdict] > priority[result.Verdict] {
			result.Verdict, result.Response, result.ResponseModel, result.Error = item.Verdict, item.Response, item.ResponseModel, item.Error
		}
	}
	if ctx.Err() != nil {
		result.Verdict, result.Error = "error", "终端续接检测已取消或任务超时"
	}
	result.DurationMS = time.Since(started).Milliseconds()
	result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result
}
