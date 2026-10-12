package accountops_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

type exclusionRepository struct {
	*business.Store
	excluded   bool
	excludedID string
}

func (r *exclusionRepository) AccountUpstreamExcluded(_ context.Context, id string) (bool, error) {
	return r.excluded && (r.excludedID == "" || r.excludedID == id), nil
}

func TestModelApplyMixedExclusionKeepsCatalogValidationAndManagedWrite(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "managed write", true: "changed catalog rejected"}[changed], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "business.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','excluded','{}','now'),('42','managed','{}','now')`)
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"41", "42"} {
				if err := store.SaveAccountModels(t.Context(), id, []string{"known"}); err != nil {
					t.Fatal(err)
				}
			}
			mapping := map[string]any{}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/admin/accounts/42" {
					t.Errorf("excluded or unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Method == http.MethodPut {
					var payload struct {
						Credentials struct {
							Mapping map[string]any `json:"model_mapping"`
						} `json:"credentials"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					mapping = payload.Credentials.Mapping
					writes++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 42, "credentials": map[string]any{"model_mapping": mapping}}})
			}))
			defer server.Close()
			repo := &exclusionRepository{Store: store, excludedID: "41"}
			tasks, runner := &settingsTasks{}, &settingsRunner{}
			service := accountops.New(settingsTarget{endpoint: server.URL}, repo, tasks)
			service.UseTaskRunner(runner)
			preview, err := service.ModelSyncPreview(t.Context(), []string{"41", "42"})
			if err != nil {
				t.Fatal(err)
			}
			repo.excluded = true
			if changed {
				if err := store.SaveAccountModels(t.Context(), "42", []string{"changed"}); err != nil {
					t.Fatal(err)
				}
			}
			_, err = service.EnqueueModelApply(t.Context(), accountops.ModelApplyRequest{Accounts: []accountops.AccountModelSelection{{AccountID: "41", Models: []string{"known"}}, {AccountID: "42", Models: []string{"known"}}}, CatalogFingerprint: preview.Fingerprint})
			if changed {
				if !errors.Is(err, accountops.ErrModelCatalogChanged) || writes != 0 {
					t.Fatalf("changed catalog: %v writes=%d", err, writes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			runner.run(t.Context())
			if tasks.last.Status != "succeeded" || writes != 1 || mapping["known"] != "known" {
				t.Fatalf("mixed apply: %+v writes=%d mapping=%v", tasks.last, writes, mapping)
			}
		})
	}
}
func TestExcludedModelDiscoveryDoesNotReadRemoteOrFailTask(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "before queue", true: "after queue"}[late], func(t *testing.T) {
			f := newSettingsFixture(t)
			repo := &exclusionRepository{Store: f.repository, excluded: !late}
			service := accountops.New(settingsTarget{endpoint: "https://must-not-contact.invalid"}, repo, f.tasks)
			service.UseTaskRunner(f.runner)
			if _, err := service.EnqueueModelDiscovery(t.Context(), []string{"41"}, "test"); err != nil {
				t.Fatal(err)
			}
			repo.excluded = true
			f.runner.run(t.Context())
			if f.tasks.last.Status != "succeeded" {
				t.Fatalf("excluded discovery: %+v", f.tasks.last)
			}
		})
	}
}

func TestModelPreviewWithAllAccountsExcludedReturnsEmptyScope(t *testing.T) {
	f := newSettingsFixture(t)
	repo := &exclusionRepository{Store: f.repository, excluded: true}
	service := accountops.New(settingsTarget{endpoint: "https://must-not-contact.invalid"}, repo, f.tasks)
	preview, err := service.ModelSyncPreview(t.Context(), []string{"41"})
	if err != nil || preview.AccountCount != 0 || len(preview.Accounts) != 0 {
		t.Fatalf("excluded preview: %+v %v", preview, err)
	}
}

func TestModelApplyExclusionAfterPreviewDoesNotFailOrContactRemote(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "before queue", true: "after queue"}[late], func(t *testing.T) {
			f := newSettingsFixture(t)
			if err := f.repository.SaveAccountModels(t.Context(), "41", []string{"known"}); err != nil {
				t.Fatal(err)
			}
			repo := &exclusionRepository{Store: f.repository}
			service := accountops.New(settingsTarget{endpoint: "https://must-not-contact.invalid"}, repo, f.tasks)
			service.UseTaskRunner(f.runner)
			preview, err := service.ModelSyncPreview(t.Context(), []string{"41"})
			if err != nil {
				t.Fatal(err)
			}
			repo.excluded = !late
			_, err = service.EnqueueModelApply(t.Context(), accountops.ModelApplyRequest{Accounts: []accountops.AccountModelSelection{{AccountID: "41", Models: []string{"known"}}}, CatalogFingerprint: preview.Fingerprint})
			if err != nil {
				t.Fatal(err)
			}
			repo.excluded = true
			f.runner.run(t.Context())
			if f.tasks.last.Status != "succeeded" || f.tasks.last.Result["remote_write"] == true {
				t.Fatalf("excluded apply: %+v", f.tasks.last)
			}
		})
	}
}
