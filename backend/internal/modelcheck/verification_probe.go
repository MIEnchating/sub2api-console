package modelcheck

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func executeVerificationProbe(ctx context.Context, directClient, oauthClient *http.Client, credential credentialResolution, input targetRequest, challenge verificationChallenge, row VerificationProbe) VerificationProbe {
	requestID := row.RequestID
	row.Status = "error"
	row.Usage = &AnimationUsage{}
	trace := &verificationTransport{base: directClient.Transport}
	direct := *directClient
	direct.Transport = trace
	oauth := *oauthClient
	if credential.oauth != nil {
		trace.base = oauthClient.Transport
		oauth.Transport = trace
	}
	var sender reasoningSender
	if credential.oauth != nil {
		sender = oauthBundleSender{client: &oauth, credential: *credential.oauth, models: []string{input.Model}, slots: make(chan struct{}, 1), requestID: requestID, stream: true, usage: row.Usage}
	} else {
		value := credential.value
		// A failed host must not silently change the path being verified.
		value.FallbackBaseURL = ""
		if value.Platform == "claude" {
			value.Platform = "anthropic"
		}
		sender = directBundleSender{client: &direct, credential: value, requestID: requestID, stream: true, usage: row.Usage}
	}
	probeStarted := time.Now()
	row.StartedAt = probeStarted.UTC().Format(time.RFC3339Nano)
	var answer, model string
	var sendErr error
	if challenge.public != nil {
		answer, model, sendErr = sendPublicReference(ctx, &direct, credential.value, input.Model, *challenge.public, requestID, row.Usage, input.TimeoutSeconds)
	} else {
		answer, model, sendErr = sender.SendWithReasoning(ctx, input.AccountID, input.Model, challenge.prompt, input.TimeoutSeconds, "medium")
	}
	row.DurationMS = time.Since(probeStarted).Milliseconds()
	row.Protocol, row.HTTPStatus, row.Attempts = trace.protocol, trace.status, trace.attempts
	row.ResponseModel = model
	if sendErr == nil && strings.TrimSpace(answer) == "" {
		sendErr = fmt.Errorf("上游未返回有效文本")
	}
	if sendErr == nil && credential.value.Secret != "" && strings.Contains(answer, credential.value.Secret) {
		sendErr = fmt.Errorf("上游回答包含敏感信息，已拒绝展示")
	}
	if sendErr != nil {
		row.Error = safeCredentialError(sendErr)
		row.ErrorCode = verificationErrorCode(row.Error, ctx, trace.status)
		row.Error += verificationErrorAdvice(row.ErrorCode)
	} else {
		row.Answer, row.AnswerTruncated = precheckAnswer(answer)
		row.ResponseModel = model
		if challenge.measurement {
			row.Status = "observed"
		} else {
			row.Status = "failed"
			if verificationAnswerMatches(challenge, answer) {
				row.Status = "passed"
			}
		}
	}
	return row
}
