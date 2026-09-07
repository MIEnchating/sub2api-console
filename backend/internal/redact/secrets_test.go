package redact

import (
	"strings"
	"testing"
)

func TestSecretsRedactsStructuredAndBearerCredentials(t *testing.T) {
	input := `password=plain access_token:"access" client_secret = top-secret Authorization: Bearer-token bearer abc.def cookie=session-value api_key=sk-key`
	result := Secrets(input)
	for _, secret := range []string{"plain", "access\"", "top-secret", "Bearer-token", "abc.def", "session-value", "sk-key"} {
		if strings.Contains(result, secret) {
			t.Fatalf("secret %q remained in %q", secret, result)
		}
	}
	if !strings.Contains(result, "password=<已隐藏>") || !strings.Contains(result, "access_token=<已隐藏>") {
		t.Fatalf("redaction labels were lost: %q", result)
	}
}

func TestSecretsRedactsCompleteCredentialValues(t *testing.T) {
	for _, input := range []string{
		`Authorization: Bearer sensitive-token`,
		`Authorization: Basic sensitive-token`,
		`{"password":"first sensitive-token last"}`,
		`password='first sensitive-token last'`,
		`{"password":"escaped \" sensitive-token"}`,
		`{"admin_key":"sensitive-token"}`,
		`{"password":"sensitive-token`,
	} {
		t.Run(input, func(t *testing.T) {
			result := Secrets(input)
			if strings.Contains(result, "sensitive-token") {
				t.Fatalf("credential remained in diagnostic: %q", result)
			}
			if !strings.Contains(result, "<已隐藏>") {
				t.Fatalf("redaction marker missing: %q", result)
			}
		})
	}
}
