package modelcheck_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestAnimationFailureKeepsUpstreamReasonWithoutCredential(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"JSON", fmt.Sprintf(`{"error":{"code":"insufficient_quota","message":%q}}`, "quota exhausted "+fixtureSecret)},
		{"Responses SSE", fmt.Sprintf("data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"insufficient_quota\",\"message\":%q}}}\n\n", "quota exhausted "+fixtureSecret)},
		{"error SSE", fmt.Sprintf("data: {\"type\":\"error\",\"error\":{\"code\":\"insufficient_quota\",\"message\":%q}}\n\n", "quota exhausted "+fixtureSecret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, tc.body) })
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if !strings.Contains(row.Error, "quota exhausted") || !strings.Contains(row.Error, "insufficient_quota") || strings.Contains(row.Error, fixtureSecret) {
				t.Fatalf("lost or unsafe reason: %s", row.Error)
			}
		})
	}
}

func TestAnimationInterruptedBodyKeepsReadError(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10000")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if !strings.Contains(row.Error, "unexpected EOF") || row.SVG != "" {
		t.Fatalf("lost read error: %#v", row)
	}
}

type failedAnimationBody struct {
	io.Reader
	failure error
}

func (b failedAnimationBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		return n, b.failure
	}
	return n, err
}
func (b failedAnimationBody) Close() error { return nil }

func TestOAuthAnimationReadTimeoutKeepsCause(t *testing.T) {
	f, _ := oauthAccountFixture(t, oauthAnimationAccount)
	f.service.UseOAuthTransport(oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: failedAnimationBody{Reader: strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"), failure: context.DeadlineExceeded}}, nil
	}))
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
	if !strings.Contains(row.Error, "超时") || !strings.Contains(row.Error, "context deadline exceeded") || row.SVG != "" {
		t.Fatalf("lost timeout: %#v", row)
	}
}

func TestOAuthAnimationFailureKeepsReasonAndRedactsBoundTokens(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, _ := oauthAccountFixture(t, oauthAnimationAccount)
			f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
				return oauthResponse(status, "application/json", fmt.Sprintf(`{"status":"failed","error":{"message":%q}}`, "rate limit exceeded "+oauthFixtureToken+" isolated-refresh-token")), nil
			}))
			if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err != nil {
				t.Fatal(err)
			}
			row := finished(t, f).Result["animations"].([]modelcheck.AnimationResult)[0]
			if !strings.Contains(row.Error, "rate limit exceeded") || strings.Contains(row.Error, oauthFixtureToken) || strings.Contains(row.Error, "isolated-refresh-token") {
				t.Fatalf("lost or unsafe error: %s", row.Error)
			}
		})
	}
}
