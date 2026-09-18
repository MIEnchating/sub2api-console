package accountworkbench

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/golang-jwt/jwt/v5"
)

type VerifiedIdentity struct {
	UserID      string
	WorkspaceID string
	Email       string
	Plan        string
}

var ErrIdentity = errors.New("官方账号身份验证失败，请重新授权并核对账号与工作区")

// VerifyCredential verifies the signed access token using only the fixed
// official key endpoint. User-supplied JWT headers cannot select a key URL.
func (s *Service) VerifyCredential(ctx context.Context, credentials map[string]any, proxyURL string) (VerifiedIdentity, error) {
	token := text(credentials["access_token"])
	if token == "" || len(token) > 32768 {
		return VerifiedIdentity{}, ErrIdentity
	}
	keys, err := s.officialSigningKeys(ctx, proxyURL)
	if err != nil {
		return VerifiedIdentity{}, err
	}
	parsed, err := jwt.Parse(token, func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || keys[kid] == nil {
			return nil, ErrIdentity
		}
		return keys[kid], nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("https://auth.openai.com"), jwt.WithAudience("https://api.openai.com/v1"), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second), jwt.WithJSONNumber())
	if err != nil || !parsed.Valid {
		return VerifiedIdentity{}, ErrIdentity
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return VerifiedIdentity{}, ErrIdentity
	}
	auth, profile := object(claims["https://api.openai.com/auth"]), object(claims["https://api.openai.com/profile"])
	result := VerifiedIdentity{UserID: text(auth["chatgpt_user_id"]), WorkspaceID: text(auth["chatgpt_account_id"]), Email: text(profile["email"]), Plan: text(auth["chatgpt_plan_type"])}
	if result.UserID == "" {
		result.UserID = text(auth["user_id"])
	}
	if result.Email == "" {
		result.Email = text(claims["email"])
	}
	if result.UserID == "" || result.WorkspaceID == "" || !validEmail(result.Email) || len(result.UserID) > 256 || len(result.WorkspaceID) > 256 || strings.ContainsAny(result.UserID+result.WorkspaceID, "\r\n\x00") {
		return VerifiedIdentity{}, ErrIdentity
	}
	workspace, user := credentialIdentity(credentials)
	if (workspace != "" && workspace != result.WorkspaceID) || (user != "" && user != result.UserID) || (text(credentials["email"]) != "" && !strings.EqualFold(text(credentials["email"]), result.Email)) {
		return VerifiedIdentity{}, ErrIdentity
	}
	return result, nil
}

type signingKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (s *Service) officialSigningKeys(ctx context.Context, proxyURL string) (map[string]*rsa.PublicKey, error) {
	if browserlogin.ValidateProxyURL(proxyURL) != nil {
		return nil, ErrIdentity
	}
	transport := s.officialTransport
	if transport == nil {
		private, err := browserlogin.NewProxyTransport(ctx, proxyURL)
		if err != nil {
			return nil, errors.New("官方签名公钥读取失败，请检查网络或代理后重试")
		}
		defer private.CloseIdleConnections()
		transport = private
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://auth.openai.com/.well-known/jwks.json", nil)
	if err != nil {
		return nil, ErrIdentity
	}
	client := http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("官方签名公钥读取失败，请检查网络或代理后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrIdentity
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, ErrIdentity
	}
	var set struct {
		Keys []signingKey `json:"keys"`
	}
	if json.Unmarshal(raw, &set) != nil || len(set.Keys) > 100 {
		return nil, ErrIdentity
	}
	result := map[string]*rsa.PublicKey{}
	for _, key := range set.Keys {
		if key.Kty != "RSA" || (key.Use != "" && key.Use != "sig") || (key.Alg != "" && key.Alg != "RS256") {
			continue
		}
		if key.Kid == "" || result[key.Kid] != nil {
			return nil, ErrIdentity
		}
		modulus, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil || len(modulus) < 256 || len(modulus) > 1024 {
			return nil, ErrIdentity
		}
		exponent, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil || len(exponent) == 0 || len(exponent) > 4 {
			return nil, ErrIdentity
		}
		e := new(big.Int).SetBytes(exponent).Int64()
		if e < 3 || e > 1<<31-1 || e%2 == 0 {
			return nil, ErrIdentity
		}
		n := new(big.Int).SetBytes(modulus)
		if n.BitLen() < 2048 {
			return nil, ErrIdentity
		}
		result[key.Kid] = &rsa.PublicKey{N: n, E: int(e)}
	}
	if len(result) == 0 {
		return nil, ErrIdentity
	}
	return result, nil
}
