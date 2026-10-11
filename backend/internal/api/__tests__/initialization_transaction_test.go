package api_test

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type initializationBusiness struct{ api.Business }

func (initializationBusiness) Bootstrap(context.Context) error { return nil }

func TestFailedInitializationSessionLeavesCredentialsRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "initialize.sqlite3")
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER reject_session BEFORE INSERT ON console_sessions BEGIN SELECT RAISE(ABORT, 'isolated session failure'); END`); err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{}, store, initializationBusiness{}, api.Dependencies{})
	request := httptest.NewRequest("POST", "http://localhost/api/setup/initialize", strings.NewReader(`{"username":"operator","password":"private-password","local_export_only":true}`))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code < 400 {
		t.Fatal("session failure accepted")
	}
	status, err := store.PublicStatus(context.Background())
	if err != nil || status.Initialized {
		t.Fatalf("failed initialization left credentials committed: initialized=%t err=%v", status.Initialized, err)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_session`); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRequest("POST", "http://localhost/api/setup/initialize", strings.NewReader(`{"username":"operator","password":"private-password","local_export_only":true}`))
	retry.RemoteAddr = "127.0.0.1:1234"
	retry.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, retry)
	if response.Code != 200 || len(response.Result().Cookies()) != 1 {
		t.Fatalf("retry failed: %d", response.Code)
	}
	user, err := store.SessionUser(context.Background(), response.Result().Cookies()[0].Value, time.Now())
	if err != nil || user == nil || *user != "operator" {
		t.Fatalf("session unavailable after retry: %v", err)
	}
}
