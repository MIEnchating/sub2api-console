package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPushURLRequiresManagementAndMatchingMonitorRevision(t *testing.T) {
	f := resourceFixture(t)
	f.monitors["19"] = json.RawMessage(`{"id":19,"name":"Push","type":"push","interval":60,"active":true,"pushToken":"private-push-token"}`)
	c := f.configure(t, false)
	if _, err := f.service.PushURL(context.Background(), 19, ""); PublicError(err).Code != "kuma_management_required" {
		t.Fatal("metrics credentials exposed push URL")
	}
	if _, err := f.service.Save(context.Background(), ConfigInput{Revision: c.Revision, BaseURL: f.server.URL, APIKey: "test-key", Username: "admin", Password: "test-password"}); err != nil {
		t.Fatal(err)
	}
	m, _, err := decodeMonitor(f.monitors["19"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PushURL(context.Background(), 19, "stale"); PublicError(err).Code != "kuma_monitor_conflict" {
		t.Fatal("stale monitor accepted")
	}
	value, err := f.service.PushURL(context.Background(), 19, m.Revision)
	if err != nil || !strings.HasSuffix(value, "/api/push/private-push-token?status=up&msg=OK&ping=") {
		t.Fatalf("push URL: %v", err)
	}
	data, _ := json.Marshal(m)
	if strings.Contains(string(data), "private-push-token") {
		t.Fatal("ordinary monitor response leaked token")
	}
}
