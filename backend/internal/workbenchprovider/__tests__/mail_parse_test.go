package workbenchprovider_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

func TestMailCandidatesDecodeHTMLAndBase64AndDeduplicateMessageIdentity(t *testing.T) {
	encodedBody := base64.StdEncoding.EncodeToString([]byte("<p>OpenAI verification code: <b>234567</b></p>"))
	payload := `{"messages":[{"id":"new","received_at":"2026-09-14T00:01:00Z","subject":"ChatGPT verification code 234567","body":"` + encodedBody + `"},{"id":"old","received_at":"2026-09-14T00:00:00Z","code":123456}]}`
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(payload))
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates = %#v, %v", candidates, err)
	}
	if candidates[0].Code != "234567" || candidates[1].Code != "123456" || candidates[0].Key == candidates[1].Key {
		t.Fatal("candidates did not preserve message identity and newest-first order")
	}
	if !candidates[0].ReceivedAt.Equal(time.Date(2026, 9, 14, 0, 1, 0, 0, time.UTC)) {
		t.Fatal("candidate lost its message timestamp")
	}
}

func TestMailCandidatesIgnorePhoneNumbersIdentifiersAndInvisibleMarkup(t *testing.T) {
	for _, payload := range []string{
		`{"id":"123456","phone":"15551234567","text":"Your order number is 654321"}`,
		`<style>.otp{content:"123456"}</style><script>"OpenAI code 234567"</script><template><script>"ChatGPT 345678"</script>verification code 456789</template><p>Phone 1234567890</p>`,
		`123456`,
		`{"text":"OpenAI verification number 1234567"}`,
	} {
		candidates, err := workbenchprovider.ExtractMailCandidates([]byte(payload))
		if err != nil || len(candidates) != 0 {
			t.Fatalf("unrelated or invisible number accepted: %#v, %v", candidates, err)
		}
	}
}

func TestMailCandidatesRequireSuccessfulBusinessResponse(t *testing.T) {
	for _, payload := range []string{
		`{"success":false,"data":{"otp":"123456"},"message":"secret-token"}`,
		`{"status":"error","data":{"otp":"123456"}}`,
		`{"code":401,"message":"verification code 123456"}`,
		`{"data":{"error":{"message":"private-password"},"otp":"123456"}}`,
	} {
		_, err := workbenchprovider.ExtractMailCandidates([]byte(payload))
		var providerError *workbenchprovider.Error
		if !errors.As(err, &providerError) || providerError.Code != "mail_service_failed" || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
			t.Fatalf("business failure was accepted or leaked: %v", err)
		}
	}
}

func TestMailCandidatesAcceptExplicitZeroErrorEnvelope(t *testing.T) {
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(`{"error":0,"data":{"otp":"123456"}}`))
	if err != nil || len(candidates) != 1 || candidates[0].Code != "123456" {
		t.Fatalf("zero-error envelope rejected: %#v, %v", candidates, err)
	}
}

func TestMailCandidatesKeepReceivedTimeWhenEnvelopeUpdateTimeIsNewer(t *testing.T) {
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(`{"id":"existing","received_at":"2026-09-13T00:00:00Z","updated_at":"2026-09-14T00:00:00Z","otp":"123456"}`))
	if err != nil || len(candidates) != 1 || candidates[0].ReceivedAt.Day() != 13 {
		t.Fatalf("update time replaced delivery time: %#v, %v", candidates, err)
	}
}

func TestMailCandidatesExtractNearbyTimestampAndLeadingZeroCode(t *testing.T) {
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(`<p>2026-09-14 01:02:03 OpenAI 验证码：<b>012345</b></p>`))
	if err != nil || len(candidates) != 1 || candidates[0].Code != "012345" || candidates[0].ReceivedAt.IsZero() {
		t.Fatalf("timestamped HTML candidate = %#v, %v", candidates, err)
	}
}

func TestMailCandidateJSONNeverSerializesCodeOrMessageFingerprint(t *testing.T) {
	candidates, err := workbenchprovider.ExtractMailCandidates([]byte(`{"otp":"123456"}`))
	if err != nil || len(candidates) != 1 {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(candidates)
	if err != nil || string(encoded) != `[{}]` {
		t.Fatalf("private candidate serialized = %s, %v", encoded, err)
	}
}

func TestMailCandidatesRejectMalformedJSONInsteadOfTreatingErrorPayloadAsEmail(t *testing.T) {
	for _, payload := range []string{`{"otp":"123456"`, `{"otp":"123456"} {"otp":"234567"}`} {
		if _, err := workbenchprovider.ExtractMailCandidates([]byte(payload)); err == nil {
			t.Fatal("malformed JSON accepted as a candidate source")
		}
	}
}
