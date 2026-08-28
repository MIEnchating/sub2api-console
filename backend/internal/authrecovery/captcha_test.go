package authrecovery

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type captchaStore struct {
	record     *configstore.AuthRecord
	entry      *configstore.VaultEntry
	saved      []configstore.AuthRecord
	savedVault []configstore.VaultEntry
}

func (s *captchaStore) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	if s.record == nil {
		return nil, nil
	}
	copy := *s.record
	copy.Headers, copy.Cookies = cloneMap(s.record.Headers), cloneMap(s.record.Cookies)
	return &copy, nil
}

func (s *captchaStore) VaultEntry(context.Context, string) (*configstore.VaultEntry, error) {
	if s.entry == nil {
		return nil, nil
	}
	copy := *s.entry
	copy.Headers = cloneMap(s.entry.Headers)
	return &copy, nil
}

func (s *captchaStore) SaveAuthRecord(_ context.Context, record configstore.AuthRecord, _ map[string]bool) error {
	s.saved = append(s.saved, record)
	return nil
}

func (s *captchaStore) SaveVaultEntry(_ context.Context, entry configstore.VaultEntry, _ map[string]bool) error {
	s.savedVault = append(s.savedVault, entry)
	return nil
}

type captchaVerifier struct {
	verified []configstore.AuthRecord
	err      error
}

func (v *captchaVerifier) Verify(_ context.Context, record configstore.AuthRecord) error {
	v.verified = append(v.verified, record)
	return v.err
}

type captchaCatalog struct {
	read int
	err  error
}

func (c *captchaCatalog) ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	c.read++
	if c.err != nil {
		return business.UpstreamCatalogSnapshot{}, c.err
	}
	return business.UpstreamCatalogSnapshot{
		Groups: []business.UpstreamCatalogGroup{{GroupID: "1", Name: "codex"}},
		Keys:   []business.UpstreamCatalogKey{{KeyID: "7", Name: "codex-key"}},
	}, nil
}

func TestCaptchaSubmitEncryptsPasswordAndCommitsOnlyAfterReadback(t *testing.T) {
	publicKey := captchaPublicKey(t)
	var loginBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/settings/public":
			http.SetCookie(writer, &http.Cookie{Name: "captcha_session", Value: "session-value", Path: "/"})
			writeCaptchaJSON(writer, `{"code":0,"data":{"turnstile_enabled":false}}`)
		case "/api/v1/auth/credential-key":
			writeCaptchaJSON(writer, `{"code":0,"data":{"algorithm":"RSA-OAEP-256+A256GCM","key_id":"key-1","public_key":"`+publicKey+`","server_time":1724457600}}`)
		case "/api/v1/auth/captcha":
			writeCaptchaJSON(writer, `{"code":0,"data":{"captcha_id":"captcha-1","image_data":"`+base64.StdEncoding.EncodeToString([]byte("png-test"))+`","expires_in":300}}`)
		case "/api/v1/auth/login":
			if request.Header.Get("Authorization") != "" || request.Header.Get("X-Site") != "custom" {
				t.Errorf("stale or missing headers: %#v", request.Header)
			}
			if request.Header.Get("X-User-UI-Request") != "1" || request.Header.Get("Origin") != "http://"+request.Host {
				t.Errorf("browser request identity missing: %#v", request.Header)
			}
			if request.Header.Get("Sec-Fetch-Site") != "same-origin" || !strings.Contains(request.UserAgent(), "Chrome/150.0.0.0") {
				t.Errorf("browser fetch identity missing: %#v", request.Header)
			}
			if cookie, err := request.Cookie("captcha_session"); err != nil || cookie.Value != "session-value" {
				t.Errorf("captcha session cookie missing: cookie=%v err=%v", cookie, err)
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(body)
			loginBody = string(encoded)
			if body["captcha_code"] != "AB12" {
				t.Errorf("captcha code was not normalized: %#v", body)
			}
			if _, present := body["email"]; present {
				t.Errorf("login request exposed plaintext email: %#v", body)
			}
			if _, present := body["password"]; present {
				t.Errorf("login request exposed plaintext password: %#v", body)
			}
			http.SetCookie(writer, &http.Cookie{Name: "sub2api_refresh_token", Value: "rotated-refresh", Path: "/"})
			writeCaptchaJSON(writer, `{"code":0,"data":{"access_token":"fresh-token","balance":"unverified"}}`)
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()

	username, password, expired := "operator@example.test", "secret-password", "expired-token"
	store := &captchaStore{
		record: &configstore.AuthRecord{
			Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
			AccessToken: &expired, Headers: map[string]string{"Authorization": "Bearer expired-token", "X-Site": "custom"}, Cookies: map[string]string{"old": "cookie"},
		},
		entry: &configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}},
	}
	verifier, catalog := &captchaVerifier{}, &captchaCatalog{}
	manager := NewCaptchaManager(store, verifier, catalog, server.Client())
	challenge, err := manager.Prepare(context.Background(), *store.record, "selected", nil)
	if err != nil {
		t.Fatal(err)
	}
	encodedChallenge, _ := json.Marshal(challenge)
	if strings.Contains(string(encodedChallenge), username) || strings.Contains(string(encodedChallenge), password) {
		t.Fatalf("public challenge leaked credentials: %s", encodedChallenge)
	}
	result, err := manager.Submit(context.Background(), challenge.ChallengeID, "ab12")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loginBody, username) || strings.Contains(loginBody, password) || strings.Contains(loginBody, expired) {
		t.Fatalf("login body leaked plaintext credentials: %s", loginBody)
	}
	if !result.Success || result.Balance != nil || result.Concurrency != nil || result.Groups != 1 || result.Keys != 1 || len(store.saved) != 1 {
		t.Fatalf("unexpected result or commit: result=%#v saved=%#v", result, store.saved)
	}
	stored := store.saved[0]
	if stored.AccessToken == nil || *stored.AccessToken != "fresh-token" || stored.RefreshToken == nil || *stored.RefreshToken != "rotated-refresh" {
		t.Fatalf("rotated credentials were not committed: %#v", stored)
	}
	if stored.Headers["User-Agent"] != captchaBrowserUserAgent || stored.Headers["Sec-Fetch-Site"] != "same-origin" || stored.Headers["Referer"] != server.URL+"/login" {
		t.Fatalf("browser session identity was not persisted: %#v", stored.Headers)
	}
	if len(verifier.verified) != 1 || catalog.read != 1 {
		t.Fatalf("authenticated readback was not completed: verifier=%d catalog=%d", len(verifier.verified), catalog.read)
	}
	if verifier.verified[0].Headers["User-Agent"] != captchaBrowserUserAgent {
		t.Fatalf("authenticated readback changed the session fingerprint: %#v", verifier.verified[0].Headers)
	}
}

func TestCaptchaSubmitDoesNotCommitWhenCatalogReadbackFails(t *testing.T) {
	publicKey := captchaPublicKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/settings/public":
			writeCaptchaJSON(writer, `{"code":0,"data":{"turnstile_enabled":false}}`)
		case "/api/v1/auth/credential-key":
			writeCaptchaJSON(writer, `{"code":0,"data":{"algorithm":"RSA-OAEP-256+A256GCM","key_id":"key-1","public_key":"`+publicKey+`","server_time":1724457600}}`)
		case "/api/v1/auth/captcha":
			writeCaptchaJSON(writer, `{"code":0,"data":{"captcha_id":"captcha-1","image_data":"`+base64.StdEncoding.EncodeToString([]byte("png-test"))+`"}}`)
		case "/api/v1/auth/login":
			writeCaptchaJSON(writer, `{"code":0,"data":{"access_token":"fresh-token"}}`)
		}
	}))
	defer server.Close()
	username, password := "operator@example.test", "secret"
	store := &captchaStore{
		record: &configstore.AuthRecord{Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", Headers: map[string]string{}, Cookies: map[string]string{}},
		entry:  &configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}},
	}
	manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{err: errors.New("catalog unavailable")}, server.Client())
	challenge, err := manager.Prepare(context.Background(), *store.record, "selected", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(context.Background(), challenge.ChallengeID, "AB12"); err == nil || !strings.Contains(err.Error(), "分组目录复核失败") {
		t.Fatalf("expected catalog failure, got %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("credentials were committed before complete readback: %#v", store.saved)
	}
}

func TestCaptchaCanCommitVerifiedCandidateWithoutPriorPrivateRecord(t *testing.T) {
	publicKey := captchaPublicKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/settings/public":
			writeCaptchaJSON(writer, `{"code":0,"data":{"turnstile_enabled":false}}`)
		case "/api/v1/auth/credential-key":
			writeCaptchaJSON(writer, `{"code":0,"data":{"algorithm":"RSA-OAEP-256+A256GCM","key_id":"key-1","public_key":"`+publicKey+`","server_time":1724457600}}`)
		case "/api/v1/auth/captcha":
			writeCaptchaJSON(writer, `{"code":0,"data":{"captcha_id":"captcha-1","image_data":"`+base64.StdEncoding.EncodeToString([]byte("png-test"))+`"}}`)
		case "/api/v1/auth/login":
			writeCaptchaJSON(writer, `{"code":0,"data":{"access_token":"fresh-token","refresh_token":"fresh-refresh"}}`)
		}
	}))
	defer server.Close()
	username, password := "operator@example.test", "secret"
	store := &captchaStore{entry: &configstore.VaultEntry{Entry: "selected", Username: &username, Password: &password, Headers: map[string]string{}}}
	manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, server.Client())
	candidate := configstore.AuthRecord{
		Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}
	challenge, err := manager.Prepare(context.Background(), candidate, "selected", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(context.Background(), challenge.ChallengeID, "AB12"); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 || store.saved[0].AccessToken == nil || *store.saved[0].AccessToken != "fresh-token" {
		t.Fatalf("verified candidate was not committed: %#v", store.saved)
	}
}

func TestManualCaptchaCredentialStaysInMemoryUntilSuccessfulSubmit(t *testing.T) {
	publicKey := captchaPublicKey(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/settings/public":
			writeCaptchaJSON(writer, `{"code":0,"data":{"turnstile_enabled":false}}`)
		case "/api/v1/auth/credential-key":
			writeCaptchaJSON(writer, `{"code":0,"data":{"algorithm":"RSA-OAEP-256+A256GCM","key_id":"key-1","public_key":"`+publicKey+`","server_time":1724457600}}`)
		case "/api/v1/auth/captcha":
			writeCaptchaJSON(writer, `{"code":0,"data":{"captcha_id":"captcha-1","image_data":"`+base64.StdEncoding.EncodeToString([]byte("png-test"))+`"}}`)
		case "/api/v1/auth/login":
			writeCaptchaJSON(writer, `{"code":0,"data":{"access_token":"fresh-token","refresh_token":"fresh-refresh"}}`)
		}
	}))
	defer server.Close()
	username, password := "operator@example.test", "secret"
	store := &captchaStore{}
	manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, server.Client())
	record := configstore.AuthRecord{
		Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_manual_login",
		Headers: map[string]string{}, Cookies: map[string]string{},
	}
	credential := configstore.VaultEntry{Entry: "operator-entry", Username: &username, Password: &password, Hosts: []string{"api.example.test"}, Headers: map[string]string{}}
	challenge, err := manager.PrepareCredential(context.Background(), record, credential, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 0 || len(store.savedVault) != 0 {
		t.Fatalf("credentials persisted before captcha submit: auth=%#v vault=%#v", store.saved, store.savedVault)
	}
	if _, err := manager.Submit(context.Background(), challenge.ChallengeID, "AB12"); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 || len(store.savedVault) != 1 || store.savedVault[0].Entry != "operator-entry" {
		t.Fatalf("verified credentials were not committed: auth=%#v vault=%#v", store.saved, store.savedVault)
	}
}

func TestCaptchaPrepareRejectsNonEmailCredentialBeforeUpstreamRequest(t *testing.T) {
	username, password := "xiaoge", "secret"
	store := &captchaStore{entry: &configstore.VaultEntry{
		Entry: "wrong-entry", Username: &username, Password: &password,
	}}
	manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid credential reached the upstream")
			return nil, errors.New("unreachable")
		}),
	})
	record := configstore.AuthRecord{
		Host: "www.xiaobaishu.org", BaseURL: "https://www.xiaobaishu.org", UpstreamType: "sub2api",
	}
	_, err := manager.Prepare(context.Background(), record, "wrong-entry", nil)
	if err == nil || !strings.Contains(err.Error(), "用户名不是有效邮箱") {
		t.Fatalf("invalid credential was not rejected: %v", err)
	}
}

func TestCredentialEnvelopeAcceptsStandardBase64PublicKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	challenge := storedChallenge{
		publicKey:    base64.StdEncoding.EncodeToString(der),
		serverOffset: 37,
		keyID:        "xiaobaishu-key",
	}
	now := time.Unix(1787811937, 0)
	envelope, err := credentialEnvelope(challenge, "operator@example.test", "secret", now)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"key_id", "encrypted_key", "iv", "ciphertext"} {
		if envelope[field] == "" {
			t.Fatalf("envelope missing %s: %#v", field, envelope)
		}
	}
	encryptedKey, err := base64.RawURLEncoding.DecodeString(envelope["encrypted_key"])
	if err != nil {
		t.Fatal(err)
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, encryptedKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	iv, err := base64.RawURLEncoding.DecodeString(envelope["iv"])
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope["ciphertext"])
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, []byte(challenge.keyID))
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["issued_at"] != float64(now.Unix()+challenge.serverOffset) {
		t.Fatalf("issued_at was not aligned at submit time: %#v", payload)
	}
}

func TestCaptchaRequestPreservesSanitizedHTTPErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"code":"CREDENTIAL_EXPIRED","message":"凭据包时间戳已过期","reason":"password=must-not-leak"}`))
	}))
	defer server.Close()

	manager := NewCaptchaManager(&captchaStore{}, &captchaVerifier{}, &captchaCatalog{}, server.Client())
	_, err := manager.request(context.Background(), server.URL, http.MethodPost, "/login", map[string]string{}, map[string]string{}, map[string]any{"opaque": true})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "凭据包时间戳已过期") || !strings.Contains(err.Error(), "CREDENTIAL_EXPIRED") {
		t.Fatalf("upstream error detail was lost: %v", err)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("upstream error leaked a secret: %v", err)
	}
}

func captchaPublicKey(t *testing.T) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(der)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func writeCaptchaJSON(writer http.ResponseWriter, payload string) {
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(payload))
}
