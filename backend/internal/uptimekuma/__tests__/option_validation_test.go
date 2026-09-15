package uptimekuma_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestTemplateOptionsRejectCombinedProtocolValues(t *testing.T) {
	for _, scenario := range []struct{ name, method, dnsType string }{
		{"combined HTTP methods", "GET|POST", "A"},
		{"combined DNS types", "GET", "A|AAAA"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			service := uptimekuma.New(store, nil)
			_, err = service.SaveTemplate(context.Background(), "", uptimekuma.TemplateInput{
				Name: "DNS", Method: scenario.method, AuthMethod: "none",
				Monitoring: &configstore.KumaTemplateMonitoring{Type: "dns", Hostname: "service.example", DNSResolver: "1.1.1.1", DNSRecordType: scenario.dnsType, Interval: 60, Timeout: 16, RetryInterval: 60, MaxRedirects: 10, AcceptedStatusCodes: []string{"200-299"}},
			})
			if err == nil || uptimekuma.PublicError(err).Code != "kuma_invalid_options" {
				t.Fatalf("combined protocol value accepted: %v", err)
			}
		})
	}
}
