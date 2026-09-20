package accountworkbench_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"net/http"
	"strings"
	"testing"
)

func TestSourceTemplateCanBeEditedWithoutChangingSourceAndResynced(t *testing.T) {
	service, _ := fixture(t, `{"data":`+sourceAccount+`}`)
	ctx := context.Background()
	source, err := service.TemplateSource(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	library, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "来源模板", SourceID: "41", SourceVersion: source.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	original := library.Items[0]
	config := original.Config
	config.Concurrency = 12
	config.RateMultiplier = "0.25"
	config.GroupIDs = []string{"8"}
	input := accountworkbench.TemplateInput{ID: original.ID, Name: "编辑后的模板", Revision: library.Revision, Config: &config}
	updated, err := service.SaveTemplate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	item := updated.Items[0]
	if item.ID != original.ID || item.SourceID != original.SourceID || item.SourceName != original.SourceName || item.SourceVersion != original.SourceVersion || item.Revision != original.Revision+1 {
		t.Fatal("editing changed source binding or template identity")
	}
	if item.Config.Concurrency != 12 || item.Config.RateMultiplier != "0.25" || item.Summary.Concurrency != "12" || len(item.Summary.Groups) != 1 || item.Summary.Groups[0].ID != "8" {
		t.Fatal("edited configuration or summary lost")
	}
	if _, err := service.SaveTemplate(ctx, input); err == nil {
		t.Fatal("stale edit accepted")
	}
	stored, err := service.Templates(ctx)
	if err != nil || stored.Revision != updated.Revision || stored.Items[0].Config.Concurrency != 12 {
		t.Fatal("failed edit changed stored template")
	}
	resynced, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{ID: original.ID, Name: item.Name, Revision: updated.Revision, SourceID: original.SourceID, SourceVersion: source.SourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if resynced.Items[0].Config.Concurrency != original.Config.Concurrency {
		t.Fatal("resync did not restore source configuration")
	}
}

func TestSourceTemplateEditReusesDisplayNamesOnlyForUnchangedIDs(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed_ids_%t", changed), func(t *testing.T) {
			payload := strings.Replace(sourceAccount, `"group_ids":[7]`, `"proxy_id":3,"proxy":{"id":3,"name":"测试代理"},"groups":[{"id":7,"name":"测试分组"}],"group_ids":[7]`, 1)
			service, _ := fixture(t, `{"data":`+payload+`}`)
			ctx := context.Background()
			source, err := service.TemplateSource(ctx, "41")
			if err != nil {
				t.Fatal(err)
			}
			library, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{Name: "来源配置", SourceID: "41", SourceVersion: source.SourceVersion})
			if err != nil {
				t.Fatal(err)
			}
			config := library.Items[0].Config
			wantProxy, wantGroup := "测试代理", "测试分组"
			if changed {
				proxy := "4"
				config.ProxyID, config.GroupIDs = &proxy, []string{"8"}
				wantProxy, wantGroup = "代理 #4", "分组 #8"
			}
			service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
				t.Error("editing a saved template must not access the upstream")
				return nil, errors.New("upstream unavailable")
			}))
			updated, err := service.SaveTemplate(ctx, accountworkbench.TemplateInput{ID: library.Items[0].ID, Name: "已编辑", Revision: library.Revision, Config: &config})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Items[0].Summary.ProxyName != wantProxy || updated.Items[0].Summary.Groups[0].Name != wantGroup {
				t.Fatal("display names do not match stable IDs")
			}
		})
	}
}
