package accountworkbench_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOfficialIdentityVerifiesSignatureBeforeAcceptingWorkspace(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := fixture(t, `{}`)
	jwks, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kid": "test-key", "kty": "RSA", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	service.UseOfficialTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://auth.openai.com/.well-known/jwks.json" || request.Header.Get("Authorization") != "" {
			t.Fatal("untrusted key destination or credential sent")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(jwks)))}, nil
	}))
	for _, scenario := range []struct {
		name           string
		issuer         string
		audience       string
		expired        bool
		badSignature   bool
		wrongWorkspace bool
		wantValid      bool
	}{
		{name: "signed identity", issuer: "https://auth.openai.com", audience: "https://api.openai.com/v1", wantValid: true},
		{name: "wrong issuer", issuer: "https://untrusted.invalid", audience: "https://api.openai.com/v1"},
		{name: "wrong audience", issuer: "https://auth.openai.com", audience: "other-client"},
		{name: "expired", issuer: "https://auth.openai.com", audience: "https://api.openai.com/v1", expired: true},
		{name: "forged signature", issuer: "https://auth.openai.com", audience: "https://api.openai.com/v1", badSignature: true},
		{name: "different workspace", issuer: "https://auth.openai.com", audience: "https://api.openai.com/v1", wrongWorkspace: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			expires := time.Now().Add(time.Hour)
			if scenario.expired {
				expires = time.Now().Add(-time.Hour)
			}
			claims := jwt.MapClaims{"iss": scenario.issuer, "aud": []string{scenario.audience}, "exp": expires.Unix(), "iat": time.Now().Add(-2 * time.Hour).Unix(), "https://api.openai.com/auth": map[string]any{"chatgpt_user_id": "official-user", "chatgpt_account_id": "official-workspace", "chatgpt_plan_type": "plus"}, "https://api.openai.com/profile": map[string]any{"email": "owner@example.test"}}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test-key"
			token.Header["jku"] = "https://untrusted.invalid/keys"
			signed, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.badSignature {
				parts := strings.Split(signed, ".")
				signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
				signature[0] ^= 1
				parts[2] = base64.RawURLEncoding.EncodeToString(signature)
				signed = strings.Join(parts, ".")
			}
			credentials := map[string]any{"access_token": signed}
			if scenario.wrongWorkspace {
				credentials["chatgpt_account_id"] = "other-workspace"
			}
			identity, err := service.VerifyCredential(context.Background(), credentials, "")
			if scenario.wantValid {
				if err != nil || identity.WorkspaceID != "official-workspace" || identity.UserID != "official-user" || identity.Email != "owner@example.test" {
					t.Fatal("signed identity rejected", err)
				}
			} else if err == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
}
