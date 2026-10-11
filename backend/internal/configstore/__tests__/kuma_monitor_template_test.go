package configstore_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestKumaMonitorTemplateLookupKeepsInstanceAndMonitorScope(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := t.Context()
	expected := configstore.KumaMonitorTemplate{MonitorID: 19, TemplateID: "template-a", Revision: 2, Name: "探活", Model: "model-a", BodyEncoding: "json"}
	for _, fixture := range []struct {
		host string
		item configstore.KumaMonitorTemplate
	}{
		{"https://first.example.test", expected},
		{"https://second.example.test", configstore.KumaMonitorTemplate{MonitorID: 19, TemplateID: "other-instance", Revision: 3}},
		{"https://first.example.test", configstore.KumaMonitorTemplate{MonitorID: 20, TemplateID: "other-monitor", Revision: 4}},
	} {
		if err := store.SaveKumaMonitorTemplate(ctx, fixture.host, fixture.item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.KumaMonitorTemplate(ctx, "https://first.example.test", 19)
	if err != nil || got != expected {
		t.Fatalf("lookup=%+v error=%v", got, err)
	}
	if err := store.DeleteKumaMonitorTemplate(ctx, "https://first.example.test", 19); err != nil {
		t.Fatal(err)
	}
	if _, err := store.KumaMonitorTemplate(ctx, "https://first.example.test", 19); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted binding: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.KumaMonitorTemplate(cancelled, "https://second.example.test", 19); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled lookup: %v", err)
	}
}

func BenchmarkKumaMonitorTemplateLookup(b *testing.B) {
	store, err := configstore.Open(filepath.Join(b.TempDir(), "config.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	ctx := b.Context()
	const host = "https://benchmark.example.test"
	for id := int64(1); id <= 1000; id++ {
		if err := store.SaveKumaMonitorTemplate(ctx, host, configstore.KumaMonitorTemplate{MonitorID: id, TemplateID: "template", Revision: 1, Name: "健康检测", Model: "test-model", BodyEncoding: "json"}); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("catalog", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			items, err := store.KumaMonitorTemplates(ctx, host)
			if err != nil || items[500].MonitorID != 500 {
				b.Fatalf("lookup: %v", err)
			}
		}
	})
	b.Run("single", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			item, err := store.KumaMonitorTemplate(ctx, host, 500)
			if err != nil || item.MonitorID != 500 {
				b.Fatalf("lookup: %v", err)
			}
		}
	})
}
