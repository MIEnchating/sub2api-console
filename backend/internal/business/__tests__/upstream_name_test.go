package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"path/filepath"
	"testing"
)

func TestUpstreamNameSourcePreservesManualNameAndCanResumeFollowing(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "names.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	name, mode := "网站A", "auto"
	input := business.UpstreamConfigurationWrite{Host: "admin.example", Name: &name, NameMode: &mode, BaseURL: "https://admin.example", AccountBaseURL: "https://models.example/v1", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}
	if _, err := store.CreateUpstreamConfiguration(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ custom, observed, mode, want string }{
		{observed: "网站B", want: "网站B"},
		{custom: "自定义", mode: "manual", observed: "网站C", want: "自定义"},
		{mode: "auto", want: "网站C"},
	} {
		if step.mode != "" {
			input.NameMode, input.Name = &step.mode, nil
			if step.custom != "" {
				input.Name = &step.custom
			}
			if _, err := store.UpdateUpstreamConfiguration(t.Context(), input); err != nil {
				t.Fatal(err)
			}
		}
		if step.observed != "" {
			if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: input.Host, Balance: &business.UpstreamBalanceObservation{SiteName: &step.observed, Status: "已读取"}}); err != nil {
				t.Fatal(err)
			}
		}
		current, err := store.UpstreamConfigurationHost(t.Context(), input.Host)
		if err != nil || current.Name != step.want || current.AccountBaseURL != input.AccountBaseURL {
			t.Fatalf("configuration=%+v err=%v want=%s", current, err, step.want)
		}
	}
}
