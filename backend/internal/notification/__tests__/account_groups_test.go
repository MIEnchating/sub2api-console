package notification_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/notification"
)

func TestAccountAlertGroups(t *testing.T) {
	for _, test := range []struct {
		name   string
		groups []string
		probe  bool
		batch  bool
		want   string
	}{
		{name: "倍率上涨显示所属分组", groups: []string{"codex"}, want: " · 分组：codex"},
		{name: "多分组按名称排序并转义", groups: []string{"pro|专线", "codex"}, want: ` · 分组：codex、pro\|专线`},
		{name: "无分组不显示空分组标签"},
		{name: "探活仅显示发生告警的分组", groups: []string{"codex", "pro"}, probe: true, want: " · 分组：codex"},
		{name: "汇总通知保留账号分组", groups: []string{"codex"}, batch: true, want: " · 分组：codex"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "alerts.sqlite3")
			repository, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repository.Close() })
			if err := repository.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			for _, statement := range []string{
				`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at) VALUES('sub2api','sub2api-notify-rules.json','{"enabled":true,"channels":[{"type":"qqbot","enabled":true}]}','now')`,
				`INSERT INTO accounts(id,name,updated_at) VALUES('927','AllinAI-Enterprise-0.5','now'),('928','AllinAI-Enterprise-0.5','now')`,
				`INSERT INTO account_groups(account_id,group_name) VALUES('928','其他账号分组')`,
			} {
				if _, err := db.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			for _, group := range test.groups {
				if _, err := db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name) VALUES('927',?)`, group); err != nil {
					t.Fatal(err)
				}
			}
			if test.probe {
				_, err = db.ExecContext(ctx, `INSERT INTO alert_incidents VALUES('console:probe:927:codex','account.probe','account','927','PROBE','firing','now','now',NULL,NULL)`)
			} else {
				err = repository.RecordAccountOperation(ctx, business.AccountOperation{
					OperationID: "rate-up", OperationType: "account.sync", State: "succeeded", Phase: "readback",
					ObjectID: "927", RemoteConfirmed: true, ReadbackConfirmed: true,
					Before: map[string]any{"rate_multiplier": "0.45"}, After: map[string]any{"rate_multiplier": "0.5"},
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			plan, err := repository.PrepareAlertDelivery(ctx, business.NotificationChannelKey("qqbot", "test-target"), true)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Pending) != 1 {
				t.Fatalf("expected one pending account alert, got %+v", plan.Pending)
			}
			if test.batch {
				plan.Pending = append(plan.Pending, business.AlertIncident{
					IncidentKey: "auth", EventType: "upstream.auth", ObjectKind: "host", ObjectID: "upstream.example", CauseCode: "AUTH", Status: "firing",
				})
			}
			message := notification.BatchMessage(plan.Pending)
			want := `账号：AllinAI-Enterprise-0.5（\#927）` + test.want + " |"
			if !strings.Contains(message, want) || strings.Contains(message, "其他账号分组") {
				t.Fatalf("expected account and its alert groups %q, got: %s", want, message)
			}
		})
	}
}
