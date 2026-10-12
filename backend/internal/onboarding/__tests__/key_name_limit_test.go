package onboarding_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
	"github.com/google/uuid"
)

type keyNameUpstream struct {
	reject  bool
	created []string
	keyName string
}

// Exercise the real HTTP client against an isolated New API token endpoint.
func keyNameService(t *testing.T, groupName, upstreamType string) (*onboarding.Service, *business.Store, onboarding.Request, *keyNameUpstream) {
	t.Helper()
	remote := &keyNameUpstream{}
	var repo *business.Store
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/":
			var body struct {
				Name string `json:"name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			remote.created = append(remote.created, body.Name)
			pending, err := repo.PendingOnboarding(r.Context(), "upstream.test", "6", []string{"3"})
			if err != nil || pending == nil || pending.UpstreamKeyName == nil || *pending.UpstreamKeyName != body.Name || !pending.KeyCommitUnknown {
				t.Errorf("create must use the persisted marker: pending=%+v err=%v name=%q", pending, err, body.Name)
			}
			if remote.reject || len(body.Name) > 50 {
				_, _ = w.Write([]byte(`{"success":false,"message":"Token name is too long"}`))
				return
			}
			remote.keyName = body.Name
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": 91, "name": body.Name, "group": "6"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{{"id": 91, "name": remote.keyName, "group": "6", "status": 1}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/91/key":
			_, _ = w.Write([]byte(`{"success":true,"data":{"key":"isolated-key"}}`))
		case r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview":
			_, _ = w.Write([]byte(`{"data":{"models":["gemini-2.5-flash"]}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			body["id"] = 77
			_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	_, repo, _, request := newService(t, server.URL, server.URL)
	request.UpstreamType = upstreamType
	if _, err := repo.UpdateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: request.Host, BaseURL: server.URL, UpstreamType: upstreamType, AuthMode: "newapi_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	platform, status, rate := "gemini", "active", "0.2"
	if _, err := repo.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: request.Host, Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: "6", Name: groupName, Platform: &platform, Status: &status, RawRate: &rate}}}}); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(t.Context(), server.URL, "isolated-admin-key", 5); err != nil {
		t.Fatal(err)
	}
	token, userID := "isolated-token", "17"
	if err := private.SaveAuthRecord(t.Context(), configstore.AuthRecord{Host: request.Host, BaseURL: server.URL, UpstreamType: upstreamType, AuthMode: "newapi_user_token", AccessToken: &token, UserID: &userID}, nil); err != nil {
		t.Fatal(err)
	}
	return onboarding.New(repo, private, upstreamsync.NewReader(server.Client()), nil), repo, request, remote
}

func TestNewAPIKeyNameFitsByteLimitAndRetainsFullUUID(t *testing.T) {
	for _, tc := range []struct{ name, group, prefix, upstreamType string }{
		{"short name", "pro", "pro", "newapi"},
		{"exact byte limit", "1234567890123", "1234567890123", "newapi"},
		{"one byte over limit", "12345678901234", "1234567890123", "newapi"},
		{"Chinese and emoji", "【⭐⭐】ClaudeCode官方渠道满血", "【⭐⭐】C", "newapi"},
		{"four byte characters", "🚀🚀🚀🚀高级组", "🚀🚀🚀", "newapi"},
		{"One API compatibility", "【⭐⭐】ClaudeCode官方渠道满血", "【⭐⭐】C", "oneapi"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, request, remote := keyNameService(t, tc.group, tc.upstreamType)
			result, err := service.Onboard(t.Context(), request)
			if err != nil || result["account_id"] != "77" {
				t.Fatalf("onboarding failed: result=%v err=%v", result, err)
			}
			if len(remote.created) != 1 {
				t.Fatalf("created names=%v", remote.created)
			}
			name := remote.created[0]
			if len(name) > 50 || !utf8.ValidString(name) || !strings.HasPrefix(name, tc.prefix+"-") {
				t.Fatalf("invalid token name: %q (%d bytes)", name, len(name))
			}
			if _, err := uuid.Parse(strings.TrimPrefix(name, tc.prefix+"-")); err != nil {
				t.Fatalf("full UUID lost: %q", name)
			}
		})
	}
}

func TestNewAPIOverlongPendingNameRetriesWithoutDuplicatingCommittedKey(t *testing.T) {
	for _, state := range []string{"explicit rejection", "uncertain commit", "confirmed key"} {
		t.Run(state, func(t *testing.T) {
			const group = "【⭐⭐】ClaudeCode官方渠道满血"
			service, repo, request, remote := keyNameService(t, group, "newapi")
			remote.reject = true
			if _, err := service.Onboard(t.Context(), request); err == nil || !strings.Contains(err.Error(), "Token name is too long") {
				t.Fatalf("expected explicit rejection: %v", err)
			}
			pending, err := repo.PendingOnboarding(t.Context(), request.Host, "6", []string{"3"})
			if err != nil || pending == nil || pending.KeyCommitUnknown {
				t.Fatalf("rejection must leave a retryable intent: %+v %v", pending, err)
			}
			uuidSuffix := strings.TrimPrefix(pending.OperationID, "account-onboarding-")
			legacyName := group + "-" + uuidSuffix
			pending.UpstreamKeyName = &legacyName
			if state == "uncertain commit" {
				pending.KeyCommitUnknown = true
			}
			if state == "confirmed key" {
				pending.UpstreamKeyID = "91"
			}
			if err := repo.SavePendingOnboarding(t.Context(), *pending); err != nil {
				t.Fatal(err)
			}
			remote.reject, remote.keyName = false, legacyName
			result, err := service.Onboard(t.Context(), request)
			if err != nil || result["account_id"] != "77" {
				t.Fatalf("retry failed: result=%v err=%v", result, err)
			}
			if state == "explicit rejection" {
				if len(remote.created) != 2 || remote.created[1] != "【⭐⭐】C-"+uuidSuffix {
					t.Fatalf("retry did not repair the rejected name: %v", remote.created)
				}
			} else if len(remote.created) != 1 || remote.keyName != legacyName {
				t.Fatalf("committed marker changed or key duplicated: %v %q", remote.created, remote.keyName)
			}
			pending, err = repo.PendingOnboarding(t.Context(), request.Host, "6", []string{"3"})
			if err != nil || pending != nil {
				t.Fatalf("completed retry left pending intent: %+v %v", pending, err)
			}
		})
	}
}
