package opstraffic_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
)

func BenchmarkSystemLogPageAccountLabels(b *testing.B) {
	store, db := accountLabelStore(b)
	metadata, err := json.Marshal(map[string]string{"description": strings.Repeat("x", 4096)})
	if err != nil {
		b.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000)
		INSERT INTO accounts(id,name,metadata_json,updated_at) SELECT x+1000,'无关账号',?,'now' FROM n`, string(metadata)); err != nil {
		b.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"items":[{"id":1,"account_id":41,"request_id":"request-1","level":"info"},{"id":2,"account_id":42,"request_id":"request-2","level":"error"}],"total":2}}`)
	}))
	defer server.Close()
	service := opstraffic.New(targetRepository{url: server.URL}, store)
	b.ReportAllocs()
	for b.Loop() {
		page, err := service.SearchSystemLogs(context.Background(), opstraffic.SystemLogQuery{Page: 1, PageSize: 20})
		if err != nil || len(page.Items) != 2 {
			b.Fatalf("system logs = %#v, %v", page, err)
		}
		if page.Items[0].AccountName == nil || *page.Items[0].AccountName != "请求账号" || page.Items[1].AccountName == nil || *page.Items[1].AccountName != "重试账号" {
			b.Fatal("system log labels changed")
		}
	}
}
