package authrecovery

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestCaptchaSubmitRejectsAuthConfigurationChangedAfterPreparation(t *testing.T) {
	tests := []struct {
		name          string
		initialRecord bool
		change        func(*captchaStore)
	}{
		{
			name: "existing URL changed", initialRecord: true,
			change: func(store *captchaStore) { store.record.BaseURL = "https://replacement.example.test" },
		},
		{
			name: "existing token changed", initialRecord: true,
			change: func(store *captchaStore) { store.record.AccessToken = stringPointer("replacement-token") },
		},
		{
			name: "existing record removed", initialRecord: true,
			change: func(store *captchaStore) { store.record = nil },
		},
		{
			name: "first credentials saved by another operation",
			change: func(store *captchaStore) {
				store.record = &configstore.AuthRecord{Host: "api.example.test", AccessToken: stringPointer("new-token")}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newCaptchaConflictServer(t)
			candidate := configstore.AuthRecord{
				Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
				AccessToken: stringPointer("expired-token"), Headers: map[string]string{}, Cookies: map[string]string{},
			}
			credential := configstore.VaultEntry{Entry: "operator", Username: stringPointer("operator@example.test"), Password: stringPointer("secret")}
			store := &captchaStore{entry: &credential}
			if test.initialRecord {
				initial := cloneAuthRecord(candidate)
				store.record = &initial
			}
			manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, server.Client())
			challenge, err := manager.PrepareCredential(context.Background(), candidate, credential, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			test.change(store)

			_, err = manager.Submit(context.Background(), challenge.ChallengeID, "AB12")

			if err == nil || !strings.Contains(err.Error(), "鉴权配置已变更") {
				t.Fatalf("stale captcha challenge should reject configuration changes, got %v", err)
			}
			if len(store.saved) != 0 || len(store.savedVault) != 0 {
				t.Fatalf("stale captcha challenge wrote credentials: auth=%d vault=%d", len(store.saved), len(store.savedVault))
			}
		})
	}
}

func TestCaptchaSubmitAllowsManualCandidateWhenStoredConfigurationIsUnchanged(t *testing.T) {
	server := newCaptchaConflictServer(t)
	store := &captchaStore{record: &configstore.AuthRecord{
		Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
		AccessToken: stringPointer("expired-token"), Headers: map[string]string{}, Cookies: map[string]string{},
	}}
	manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, server.Client())
	candidate := cloneAuthRecord(*store.record)
	candidate.AuthMode = "sub2api_manual_login"
	credential := configstore.VaultEntry{Entry: "operator", Username: stringPointer("operator@example.test"), Password: stringPointer("secret")}
	challenge, err := manager.PrepareCredential(context.Background(), candidate, credential, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := manager.Submit(context.Background(), challenge.ChallengeID, "AB12")

	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || len(store.saved) != 1 || pointerOr(store.saved[0].AccessToken, "") != "fresh-token" {
		t.Fatalf("manual login candidate was not committed: result=%#v writes=%d", result, len(store.saved))
	}
}

func TestCaptchaSubmitRejectsVaultEntryChangedBeforeSavingVerifiedCredentials(t *testing.T) {
	tests := []struct {
		name         string
		initialEntry bool
		change       func(*captchaStore)
	}{
		{
			name: "saved entry changed", initialEntry: true,
			change: func(store *captchaStore) { store.entry.Password = stringPointer("replacement-password") },
		},
		{
			name: "saved entry removed", initialEntry: true,
			change: func(store *captchaStore) { store.entry = nil },
		},
		{
			name: "new entry created by another operation",
			change: func(store *captchaStore) {
				store.entry = &configstore.VaultEntry{Entry: "operator", Username: stringPointer("another@example.test"), Password: stringPointer("another-password")}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newCaptchaConflictServer(t)
			store := &captchaStore{record: &configstore.AuthRecord{
				Host: "api.example.test", BaseURL: server.URL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token",
				Headers: map[string]string{}, Cookies: map[string]string{},
			}}
			credential := configstore.VaultEntry{Entry: "operator", Username: stringPointer("operator@example.test"), Password: stringPointer("secret")}
			if test.initialEntry {
				entry := cloneVaultEntry(credential)
				store.entry = &entry
			}
			manager := NewCaptchaManager(store, &captchaVerifier{}, &captchaCatalog{}, server.Client())
			challenge, err := manager.PrepareCredential(context.Background(), *store.record, credential, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			test.change(store)

			_, err = manager.Submit(context.Background(), challenge.ChallengeID, "AB12")

			if err == nil || !strings.Contains(err.Error(), "密码箱项已变更") {
				t.Fatalf("stale captcha challenge should reject vault entry changes, got %v", err)
			}
			if len(store.saved) != 0 || len(store.savedVault) != 0 {
				t.Fatalf("stale captcha challenge wrote credentials: auth=%d vault=%d", len(store.saved), len(store.savedVault))
			}
		})
	}
}

func newCaptchaConflictServer(t *testing.T) *httptest.Server {
	t.Helper()
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
		default:
			t.Errorf("unexpected captcha request: %s", request.URL.Path)
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
