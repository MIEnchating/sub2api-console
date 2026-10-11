package onboarding

import (
	"encoding/json"
	"net/http"
	"strings"
)

// New API can reject a native protocol before dispatching to its channel.
// Restrict compatibility retries to these known validation/conversion errors;
// a generic HTTP 500 or an upstream generation failure must never be replayed.
func gatewayRequiresChat(path string, status int, raw []byte) bool {
	if path != "/v1/responses" && path != "/v1/messages" {
		return false
	}
	var response struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Error.Type != "new_api_error" {
		return false
	}
	message := strings.TrimSpace(response.Error.Message)
	if before, suffix, found := strings.Cut(message, " (request id: "); found && strings.HasSuffix(suffix, ")") {
		message = before
	}
	if status == http.StatusBadRequest && path == "/v1/responses" {
		return response.Error.Code == "" && message == "field messages is required"
	}
	return status == http.StatusInternalServerError && response.Error.Code == "convert_request_failed" && message == "not implemented"
}
