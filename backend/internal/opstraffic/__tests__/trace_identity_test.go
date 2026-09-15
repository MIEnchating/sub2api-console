package opstraffic_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/opstraffic"
)

type targetRepository struct{ url string }

func (r targetRepository) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: r.url, AdminKey: "fixture", TimeoutSeconds: 5}, nil
}

type accountRepository struct{}

func (accountRepository) Accounts(context.Context) ([]business.AccountStatus, error) {
	return nil, nil
}

func (accountRepository) Account(_ context.Context, id string) (*business.AccountDetail, error) {
	return &business.AccountDetail{AccountStatus: business.AccountStatus{ID: id, Name: "账号 " + id}}, nil
}

func TestRequestTraceWithMultipleAccountsKeepsEachRecordAccountName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("kind") == "error" {
			_, _ = io.WriteString(writer, `{"data":{"items":[],"total":0}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"data":{"items":[{"request_id":"request-1","kind":"error","account_id":41},{"request_id":"request-1","kind":"success","account_id":42}],"total":2}}`)
	}))
	defer server.Close()
	service := opstraffic.New(targetRepository{url: server.URL}, accountRepository{})
	trace, err := service.RequestTrace(context.Background(), "request-1")
	if err != nil || len(trace.Records) != 2 {
		t.Fatalf("trace failed: %#v, %v", trace, err)
	}
	for _, record := range trace.Records {
		if record.AccountID == nil || record.AccountName == nil || *record.AccountName != "账号 "+*record.AccountID {
			t.Fatalf("trace record name belongs to a different account: %#v", record)
		}
	}
}
