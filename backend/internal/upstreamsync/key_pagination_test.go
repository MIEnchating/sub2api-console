package upstreamsync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestListKeysReadsAllPagesWhenUpstreamCapsPageSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("page") == "1" {
			_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":17,"name":"first"}],"total":2}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"success":true,"data":{"items":[{"id":18,"name":"second"}],"total":2}}`))
	}))
	defer server.Close()
	keys, err := NewReader(server.Client()).ListKeys(context.Background(), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"})
	if err != nil || len(keys) != 2 || keys[0].KeyID != "17" || keys[1].KeyID != "18" {
		t.Fatalf("server-capped pages lost stable keys: keys=%#v err=%v", keys, err)
	}
}

func TestListKeysRejectsIncompleteOrMalformedCatalog(t *testing.T) {
	for _, test := range []struct {
		name  string
		first string
		next  string
	}{
		{"missing collection", `{"success":true}`, `{"success":true}`},
		{"early empty page", `{"data":{"items":[{"id":17}],"total":2}}`, `{"data":{"items":[],"total":2}}`},
		{"repeated page", `{"data":{"items":[{"id":17}],"total":2}}`, `{"data":{"items":[{"id":17}],"total":2}}`},
		{"changing total", `{"data":{"items":[{"id":17}],"total":2}}`, `{"data":{"items":[{"id":18}],"total":3}}`},
		{"invalid empty total", `{"data":{"items":[],"total":-1}}`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				payload := test.first
				if request.URL.Query().Get("page") != "1" {
					payload = test.next
				}
				_, _ = fmt.Fprint(writer, payload)
			}))
			defer server.Close()
			keys, err := NewReader(server.Client()).ListKeys(context.Background(), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "newapi"})
			if err == nil || keys != nil {
				t.Fatalf("untrustworthy catalog accepted as complete: keys=%#v err=%v", keys, err)
			}
		})
	}
}
