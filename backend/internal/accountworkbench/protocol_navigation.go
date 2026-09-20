package accountworkbench

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func (p *protocolClient) completedLogin(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && protocolEndpoint(rawURL) && u.Host == "chatgpt.com" && u.Path == "/" &&
		p.hasCookie(protocolChatGPT, "__Secure-next-auth.session-token")
}

// Never return arbitrary URLs or page contents: even their paths can contain
// private identifiers. Only these known stages are safe to describe publicly.
func protocolUnexpectedLoginPage(rawURL string) error {
	switch protocolPath(rawURL) {
	case "/authorize", "/oauth/authorize":
		return errors.New("官方登录停留在授权入口，未进入密码或邮箱验证页面，请重新授权；若持续出现请反馈此阶段")
	case "/log-in", "/log-in-or-create-account":
		return errors.New("官方登录停留在账号登录入口，未进入密码或邮箱验证页面，请核对邮箱后重新授权")
	case "/about-you", "/add-phone", "/create-account":
		return errors.New("官方要求补充账号资料，请在官方站点完成后重新授权")
	default:
		return errors.New("官方返回了未识别的登录页面，本次授权已停止，请重新授权；若持续出现请反馈此提示")
	}
}

var protocolHTMLPattern = regexp.MustCompile(`(?i)<(?:!doctype\s+html|html|head|body)\b`)
var protocolChallengePattern = regexp.MustCompile(`(?i)Just a moment|cdn-cgi|challenge-platform|cf-challenge`)

func protocolSecurityChallenge(status int, headers http.Header, body []byte) bool {
	for _, key := range []string{"Cf-Mitigated", "X-Cf-Mitigated"} {
		if strings.Contains(strings.ToLower(headers.Get(key)), "challenge") {
			return true
		}
	}
	isHTML := strings.Contains(strings.ToLower(headers.Get("Content-Type")), "text/html")
	if status == http.StatusForbidden {
		return isHTML || protocolChallengePattern.Match(body)
	}
	if status == http.StatusBadRequest || status == http.StatusConflict {
		return (isHTML || protocolHTMLPattern.Match(body)) && protocolChallengePattern.Match(body)
	}
	return false
}
