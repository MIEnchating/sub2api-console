package upstreamsync

import (
	"errors"
	"strings"
)

var errBusinessAuthentication = errors.New("upstream business authentication failed")

type businessResponseError struct {
	detail         string
	authentication bool
}

func (e *businessResponseError) Is(target error) bool {
	return target == errBusinessAuthentication && e.authentication
}

func (e *businessResponseError) Error() string {
	message := "上游业务读取失败"
	if e.authentication {
		message = "上游鉴权失败"
	}
	if e.detail != "" {
		message += "：" + e.detail
	}
	if e.authentication {
		message += "；请在“鉴权恢复”中重新登录或更新 Token，并核对 User ID 和账号权限"
	}
	return message
}

func newAPIAuthenticationFailure(payload map[string]any) bool {
	code, _ := payload["code"].(string)
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "AUTH_TOKEN_EXPIRED", "AUTH_SESSION_REVOKED", "AUTH_UNAUTHORIZED",
		"AUTH_USER_DISABLED", "AUTH_USER_INVALID", "AUTH_INSUFFICIENT_PRIVILEGE":
		return true
	}

	// Legacy New API returns HTTP 200 without a code for these dashboard auth
	// failures. Match its published translations exactly, not arbitrary token text.
	message, _ := payload["message"].(string)
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "unauthorized, not logged in and no access token provided",
		"unauthorized, invalid access token",
		"unauthorized, invalid user info",
		"unauthorized, new-api-user header not provided",
		"unauthorized, new-api-user header format error",
		"unauthorized, new-api-user does not match logged in user",
		"user has been banned",
		"unauthorized, insufficient privileges",
		"无权进行此操作，未登录且未提供 access token",
		"无权进行此操作，access token 无效",
		"无权进行此操作，用户信息无效",
		"无权进行此操作，未提供 new-api-user",
		"无权进行此操作，new-api-user 格式错误",
		"无权进行此操作，new-api-user 与登录用户不匹配",
		"用户已被封禁",
		"无权进行此操作，权限不足",
		"無權進行此操作，未登入且未提供 access token",
		"無權進行此操作，access token 無效",
		"無權進行此操作，使用者資訊無效",
		"無權進行此操作，未提供 new-api-user",
		"無權進行此操作，new-api-user 格式錯誤",
		"無權進行此操作，new-api-user 與登入使用者不匹配",
		"使用者已被封禁",
		"無權進行此操作，權限不足":
		return true
	default:
		return false
	}
}
