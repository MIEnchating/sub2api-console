package accountops_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

type unexpectedSyncProbe struct{ calls int }

func (p *unexpectedSyncProbe) RunNow(context.Context, probe.Request) (probe.RunSummary, error) {
	p.calls++
	return probe.RunSummary{}, nil
}

func TestManualModelSyncWithoutProbeWritesAndReadsBackWithoutCallingProbe(t *testing.T) {
	for _, withRunner := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable probe", true: "configured probe"}[withRunner], func(t *testing.T) {
			f := newSettingsFixture(t)
			ctx := context.Background()
			if err := f.repository.SaveAccountModels(ctx, "41", []string{"known-model"}); err != nil {
				t.Fatal(err)
			}
			preview, err := f.repository.AccountModelSyncPreview(ctx, []string{"41"})
			if err != nil {
				t.Fatal(err)
			}
			mapping := map[string]any{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					var payload struct {
						Credentials struct {
							Mapping map[string]any `json:"model_mapping"`
						} `json:"credentials"`
					}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					mapping = payload.Credentials.Mapping
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "credentials": map[string]any{"model_mapping": mapping}}})
			}))
			defer server.Close()
			service := accountops.New(settingsTarget{endpoint: server.URL}, f.repository, f.tasks)
			service.UseTaskRunner(f.runner)
			runner := &unexpectedSyncProbe{}
			if withRunner {
				service.UseModelSyncProbe(runner)
			}
			_, err = service.EnqueueModelApply(ctx, accountops.ModelApplyRequest{
				Accounts:           []accountops.AccountModelSelection{{AccountID: "41", Models: []string{"custom-model"}, ManualModels: []string{"custom-model"}}},
				CatalogFingerprint: preview.Fingerprint, ProbeModels: []string{}, Actor: "test",
			})
			if err != nil {
				t.Fatal(err)
			}
			f.runner.run(ctx)
			if f.tasks.last.Status != "succeeded" || f.tasks.last.Result["probe_disabled"] != true || runner.calls != 0 {
				t.Fatalf("task=%+v probe calls=%d", f.tasks.last, runner.calls)
			}
			if mapping["custom-model"] != "custom-model" {
				t.Fatalf("mapping=%v", mapping)
			}
		})
	}
}

func TestManualModelSyncRejectsWildcardAndUnmarkedUnknownModel(t *testing.T) {
	f := newSettingsFixture(t)
	ctx := context.Background()
	if err := f.repository.SaveAccountModels(ctx, "41", []string{"known"}); err != nil {
		t.Fatal(err)
	}
	preview, err := f.repository.AccountModelSyncPreview(ctx, []string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []accountops.AccountModelSelection{
		{AccountID: "41", Models: []string{"unknown"}},
		{AccountID: "41", Models: []string{"*"}, ManualModels: []string{"*"}},
		{AccountID: "41", Models: []string{"model?"}, ManualModels: []string{"model?"}},
	} {
		if _, err := f.service.EnqueueModelApply(ctx, accountops.ModelApplyRequest{Accounts: []accountops.AccountModelSelection{selection}, CatalogFingerprint: preview.Fingerprint}); err == nil {
			t.Fatalf("accepted invalid selection %+v", selection)
		}
	}
}
