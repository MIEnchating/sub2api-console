package browserlogin

import (
	"encoding/json"
	"regexp"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
)

var challengeError = regexp.MustCompile(`^\[Cloudflare Turnstile\] Error: ([0-9]{6})\.?$`)

// Keep only the documented numeric code, never console messages or arguments.
func challengeCode(browser any) string {
	reporter, ok := browser.(interface{ ChallengeCode() string })
	if !ok {
		return ""
	}
	code := reporter.ChallengeCode()
	if len(code) != 6 {
		return ""
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	return code
}

func (b *chromiumBrowser) ChallengeCode() string {
	if code := b.challenge.Load(); code != nil {
		return *code
	}
	return ""
}

func (b *chromiumBrowser) observeChallenge(event any) {
	switch value := event.(type) {
	case *page.EventFrameNavigated:
		if value.Frame != nil && value.Frame.ParentID == "" {
			b.challenge.Store(nil)
		}
	case *runtime.EventConsoleAPICalled:
		if value.Type != runtime.APITypeError && value.Type != runtime.APITypeWarning {
			return
		}
		for _, arg := range value.Args {
			if arg == nil || len(arg.Value) > 128 {
				continue
			}
			var message string
			if json.Unmarshal(arg.Value, &message) != nil {
				continue
			}
			match := challengeError.FindStringSubmatch(message)
			if len(match) == 2 {
				b.challenge.Store(&match[1])
			}
		}
	}
}
