package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func saveExportProfile(t *testing.T, f *importFixture, id string) accountworkbench.LoginProfileView {
	t.Helper()
	profile, err := f.service.SaveLoginProfile(context.Background(), "profile-owner", accountworkbench.LoginProfileSaveInput{
		AccountID: id, Confirmed: true, Login: accountworkbench.OAuthLoginInput{
			Email: "owner@example.com", Password: "private-profile-password", TOTPSecret: securitySecret,
			ProxyURL: "http://operator-{session}:proxy-private@proxy.example:8080",
			SMS:      &accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key", Country: "6", MaxPrice: "0.123456789123456789", Confirmed: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func profileExportPreview(t *testing.T, f *importFixture, profiles ...accountworkbench.LoginProfileView) accountworkbench.ProfileExportPreview {
	t.Helper()
	input := accountworkbench.ProfileExportInput{Items: make([]accountworkbench.ProfileExportSelection, 0, len(profiles))}
	for _, profile := range profiles {
		input.Items = append(input.Items, accountworkbench.ProfileExportSelection{ID: profile.ID, Revision: profile.Revision})
	}
	view, err := f.service.PreviewProfileExport(context.Background(), "profile-owner", input)
	if err != nil || view.ID == "" {
		t.Fatalf("profile export preview = %+v, %v", view, err)
	}
	return view
}

func completeProfileExport(t *testing.T, f *importFixture, view accountworkbench.ProfileExportPreview) accountworkbench.ExportMetadata {
	t.Helper()
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" || task.Operation != "account-workbench-profile-export" {
		t.Fatalf("profile export task = %+v", task)
	}
	rows, ok := task.Result["items"].([]accountworkbench.ResultItem)
	if !ok || len(rows) != 1 || rows[0].Report["kind"] != accountworkbench.ExportLoginProfiles {
		t.Fatalf("profile export task metadata = %+v", task.Result)
	}
	artifacts, err := f.service.Exports(context.Background(), "profile-owner")
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("profile export artifacts = %+v, %v", artifacts, err)
	}
	return artifacts[0]
}

func TestProfileExportWritesPrivateOriginalLoginDataWithoutRemoteRequestsOrPublicSecrets(t *testing.T) {
	f, directory := exportFixture(t)
	first, second := saveExportProfile(t, f, "101"), saveExportProfile(t, f, "102")
	var requests atomic.Int32
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("profile export must not contact remote services")
	}))
	view := profileExportPreview(t, f, second, first)
	if view.Kind != accountworkbench.ExportLoginProfiles || len(view.Items) != 2 || view.Items[0].ID != second.ID || view.Items[0].Revision != second.Revision {
		t.Fatalf("profile scope = %+v", view)
	}
	metadata := completeProfileExport(t, f, view)
	if metadata.Kind != accountworkbench.ExportLoginProfiles || metadata.Count != 2 {
		t.Fatalf("profile metadata = %+v", metadata)
	}
	payload, err := os.ReadFile(filepath.Join(directory, metadata.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type       string `json:"type"`
		Version    int    `json:"version"`
		ExportedAt string `json:"exported_at"`
		Profiles   []struct {
			ID        string          `json:"id"`
			AccountID string          `json:"account_id"`
			Login     json.RawMessage `json:"login"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "account-workbench-login-profiles" || data.Version != 1 || data.ExportedAt != metadata.CreatedAt || len(data.Profiles) != 2 || data.Profiles[0].ID != second.ID || data.Profiles[0].AccountID != "102" {
		t.Fatal("profile file lost the reviewed identity or transfer envelope")
	}
	var login accountworkbench.OAuthLoginInput
	if err := json.Unmarshal(data.Profiles[0].Login, &login); err != nil {
		t.Fatal(err)
	}
	if login.Password != "private-profile-password" || login.TOTPSecret != securitySecret || login.ProxyURL != "http://operator-{session}:proxy-private@proxy.example:8080" || login.SMS == nil || login.SMS.APIKey != "private-sms-key" || login.SMS.MaxPrice != "0.123456789123456789" || login.SMS.Confirmed {
		t.Fatal("private profile export changed stored credential content or restored stale purchase confirmation")
	}
	history, err := f.service.History(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := os.ReadFile(filepath.Join(directory, metadata.ID+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal([]any{view, metadata, history, string(sidecar)})
	for _, secret := range []string{"private-profile-password", securitySecret, "proxy-private", "private-sms-key", "test-admin-key"} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("profile metadata exposed %s", secret)
		}
	}
	for _, path := range []string{filepath.Join(directory, metadata.ID+".json"), filepath.Join(directory, metadata.ID+".meta.json")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("profile artifact permissions = %v, %v", info, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("profile export made %d remote requests", requests.Load())
	}
}

func TestProfileExportRequiresConfirmationOwnerAndCorrectKindWithoutConsumingPreview(t *testing.T) {
	f, _ := exportFixture(t)
	view := profileExportPreview(t, f, saveExportProfile(t, f, "101"))
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, false); err == nil {
		t.Fatal("unconfirmed profile export started")
	}
	if _, err := f.service.ExportProfiles(context.Background(), "another-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("other owner profile export = %v", err)
	}
	if _, err := f.service.Export(context.Background(), "profile-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("profile preview accepted by online export = %v", err)
	}
	completeProfileExport(t, f, view)
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", view.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("consumed profile preview = %v", err)
	}
	online, err := f.service.PreviewExport(context.Background(), "profile-owner", accountworkbench.ExportPreviewInput{AccountIDs: []string{"101"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportProfiles(context.Background(), "profile-owner", online.ID, true); !errors.Is(err, accountworkbench.ErrExportPreview) {
		t.Fatalf("account preview accepted by profile export = %v", err)
	}
	if _, err := f.service.Export(context.Background(), "profile-owner", online.ID, true); err != nil {
		t.Fatalf("wrong-kind attempt consumed account preview: %v", err)
	}
	if task := f.await(t); task.Status != "succeeded" {
		t.Fatalf("original account export = %+v", task)
	}
}

func TestProfileExportPreviewRejectsMissingDuplicateAndStaleSelections(t *testing.T) {
	f, _ := exportFixture(t)
	profile := saveExportProfile(t, f, "101")
	valid := accountworkbench.ProfileExportSelection{ID: profile.ID, Revision: profile.Revision}
	for _, items := range [][]accountworkbench.ProfileExportSelection{nil, {{ID: "missing", Revision: 1}}, {{ID: profile.ID, Revision: 0}}, {{ID: profile.ID, Revision: profile.Revision + 1}}, {valid, valid}} {
		if _, err := f.service.PreviewProfileExport(context.Background(), "profile-owner", accountworkbench.ProfileExportInput{Items: items}); err == nil {
			t.Fatal("invalid profile selection returned an executable preview")
		}
	}
}
