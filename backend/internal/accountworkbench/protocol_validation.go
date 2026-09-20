package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"strings"
)

type protocolRejection struct{ kind string }

func (e *protocolRejection) Error() string {
	switch e.kind {
	case "password":
		return "账号密码错误，请重新输入密码"
	case "email_code":
		return "邮箱验证码错误，请重新输入验证码"
	default:
		return "2FA 验证码错误，请输入验证器当前显示的验证码"
	}
}

var passwordRejected = regexp.MustCompile(`(?i)(?:invalid|incorrect|wrong)[_\s-]*password|password[_\s-]*(?:invalid|incorrect|wrong)`)
var totpRejected = regexp.MustCompile(`(?i)(?:invalid|incorrect|wrong)[_\s-]*(?:totp|otp|code)|(?:totp|otp|code)[_\s-]*(?:invalid|incorrect|wrong)`)
var emailRejected = regexp.MustCompile(`(?i)wrong_email_otp_code|wrong code|invalid (?:email )?(?:otp )?code|incorrect (?:email )?(?:otp )?code`)

// Read only recognized error fields for classification. Never return or persist
// upstream descriptions: rejected responses may echo credentials and codes.
func protocolError(status int, body []byte, path string) error {
	if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusUnprocessableEntity {
		var payload struct {
			Error   json.RawMessage `json:"error"`
			Code    string          `json:"code"`
			Message string          `json:"message"`
		}
		if json.Unmarshal(body, &payload) == nil {
			values := []string{payload.Code, payload.Message}
			var textError string
			if json.Unmarshal(payload.Error, &textError) == nil {
				values = append(values, textError)
			} else {
				var nested struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				}
				if json.Unmarshal(payload.Error, &nested) == nil {
					values = append(values, nested.Code, nested.Message)
				}
			}
			text := strings.Join(values, " ")
			switch {
			case path == "/api/accounts/password/verify" && passwordRejected.MatchString(text):
				return &protocolRejection{kind: "password"}
			case path == "/api/accounts/email-otp/validate" && emailRejected.MatchString(text):
				return &protocolRejection{kind: "email_code"}
			case path == "/api/accounts/mfa/verify" && totpRejected.MatchString(text):
				return &protocolRejection{kind: "totp_code"}
			}
		}
	}
	return fmt.Errorf("官方登录拒绝了本次请求（HTTP %d），请核对登录资料；请求不会自动重发", status)
}

type protocolFactor struct {
	kind, flow, device, path, referer, field, initial string
	fields                                            map[string]any
}

func (s *Service) verifyProtocolFactor(ctx context.Context, client *protocolClient, run *privateRun, index int, active *activeRun, factor protocolFactor) (map[string]any, error) {
	input := LoginInput{Value: factor.initial}
	message := ""
	for {
		if input.Value == "" {
			var err error
			input, err = s.waitLoginInputWithMessage(ctx, run, index, active, factor.kind, message)
			if err != nil {
				return nil, err
			}
		}
		if input.Action == "resend_email" {
			response, err := client.request(ctx, http.MethodPost, protocolAuth+"/api/accounts/email-otp/resend", protocolJSON(map[string]any{}), protocolHeaders(protocolAuth+"/email-verification"))
			if err != nil {
				return nil, err
			}
			if _, err := protocolPayload(response.Body); err != nil {
				return nil, err
			}
			input = LoginInput{}
			message = "验证码已重新发送，请输入邮箱收到的新验证码"
			continue
		}
		payload := maps.Clone(factor.fields)
		if payload == nil {
			payload = map[string]any{}
		}
		payload[factor.field] = input.Value
		response, err := client.verify(ctx, factor.flow, factor.device, factor.path, payload, factor.referer)
		if err == nil {
			result, parseErr := protocolPayload(response.Body)
			if parseErr != nil {
				return nil, parseErr
			}
			if factor.kind == "password" && input.Value != factor.initial {
				run.Items[index].LoginPassword = input.Value
				if saveErr := s.persistRun(run); saveErr != nil {
					return nil, errors.New("已验证密码保存失败，已停止授权")
				}
			}
			return result, nil
		}
		var rejected *protocolRejection
		if !errors.As(err, &rejected) || rejected.kind != factor.kind {
			return nil, err
		}
		// Only a definitive rejection permits a new manual submission in this session.
		// Never regenerate a rejected factor or replay an uncertain network request.
		input = LoginInput{}
		message = rejected.Error()
	}
}
