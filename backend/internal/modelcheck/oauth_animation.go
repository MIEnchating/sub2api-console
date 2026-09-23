package modelcheck

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) runOAuthAnimationTarget(ctx context.Context, account selectedAccount, timeout int, questions []string, result *AnimationResult) error {
	guarded, release, err := s.acquirePreparedAccounts(ctx, []selectedAccount{account})
	if err != nil {
		return err
	}
	defer func() { _ = release() }()
	store, ok := s.credentials.(targetguard.Store)
	if !ok {
		return errors.New("OAuth 账号检测的管理目标尚未配置")
	}
	guarded, err = targetguard.Pin(guarded, store)
	if err != nil {
		return errors.New("管理目标在检测排队后已变化或不可用，请重新提交")
	}
	credential, err := s.resolveOAuthCredential(guarded, account, true)
	if err != nil {
		return err
	}
	if credential.previewClient != nil {
		return runManagedOAuthAnimation(guarded, credential.previewClient, account, timeout, questions, result)
	}
	s.profilesMu.RLock()
	transport := s.oauthTransport
	s.profilesMu.RUnlock()
	if transport == nil {
		direct := newOAuthDirectTransport(timeout)
		defer direct.CloseIdleConnections()
		transport = direct
	}
	sender := oauthBundleSender{
		client: &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		credential: *credential, models: []string{result.Model}, slots: make(chan struct{}, 1), requestID: result.RequestID,
	}
	// Animation and precheck accept an explicit model independently of behavior profiles.
	effort := "none"
	if result.Model == astraModel {
		effort = "low"
	}
	if result.Mode == precheckMode {
		return runPrecheckQuestions(guarded, questions, result, func(question AstraQuestion, requestID string) (string, string, error) {
			sender.requestID = requestID
			return sender.SendWithReasoning(guarded, account.ID, result.Model, question.Question, timeout, effort)
		})
	}
	sender.animationStream = true
	text, model, err := sender.SendWithReasoning(guarded, account.ID, result.Model, animationPrompt, timeout, effort)
	if err != nil {
		return err
	}
	svg, err := sanitizeAnimationSVG(text)
	if err != nil {
		return err
	}
	result.SVG, result.ResponseModel = svg, safeCredentialText(model)
	return nil
}

func runManagedOAuthAnimation(ctx context.Context, client *adminclient.Client, account selectedAccount, timeout int, questions []string, result *AnimationResult) error {
	effort := "none"
	if result.Model == astraModel {
		effort = "low"
	}
	send := func(prompt, requestID string) (string, string, error) {
		response, err := client.GenerateAccountPreview(ctx, account.ID, adminclient.AccountPreviewRequest{
			ModelID: result.Model, Prompt: prompt, ReasoningEffort: effort, RequestID: requestID, TimeoutSeconds: timeout,
		})
		if err != nil {
			return "", "", err
		}
		return response.Text, safeCredentialText(response.Model), nil
	}
	if result.Mode == precheckMode {
		return runPrecheckQuestions(ctx, questions, result, func(question AstraQuestion, requestID string) (string, string, error) {
			return send(question.Question, requestID)
		})
	}
	text, model, err := send(animationPrompt, result.RequestID)
	if err != nil {
		return err
	}
	svg, err := sanitizeAnimationSVG(text)
	if err != nil {
		return err
	}
	result.SVG, result.ResponseModel = svg, model
	return nil
}

func (s *Service) runOAuthPromptTarget(ctx context.Context, account selectedAccount, timeout int, model, requestID, prompt string) (string, string, error) {
	guarded, release, err := s.acquirePreparedAccounts(ctx, []selectedAccount{account})
	if err != nil {
		return "", "", err
	}
	defer func() { _ = release() }()
	store, ok := s.credentials.(targetguard.Store)
	if !ok {
		return "", "", errors.New("OAuth 账号检测的管理目标尚未配置")
	}
	guarded, err = targetguard.Pin(guarded, store)
	if err != nil {
		return "", "", errors.New("管理目标在检测排队后已变化或不可用，请重新提交")
	}
	credential, err := s.resolveOAuthCredential(guarded, account, true)
	if err != nil {
		return "", "", err
	}
	effort := "none"
	if model == astraModel {
		effort = "low"
	}
	if credential.previewClient != nil {
		response, err := credential.previewClient.GenerateAccountPreview(guarded, account.ID, adminclient.AccountPreviewRequest{
			ModelID: model, Prompt: prompt, ReasoningEffort: effort, RequestID: requestID, TimeoutSeconds: timeout,
		})
		if err != nil {
			return "", "", err
		}
		return response.Text, safeCredentialText(response.Model), nil
	}
	s.profilesMu.RLock()
	transport := s.oauthTransport
	s.profilesMu.RUnlock()
	if transport == nil {
		direct := newOAuthDirectTransport(timeout)
		defer direct.CloseIdleConnections()
		transport = direct
	}
	sender := oauthBundleSender{
		client: &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		credential: *credential, models: []string{model}, slots: make(chan struct{}, 1), requestID: requestID,
	}
	return sender.SendWithReasoning(guarded, account.ID, model, prompt, timeout, effort)
}
