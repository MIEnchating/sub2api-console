package accountworkbench_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestPreviewWithFailedRefreshTokenDoesNotRetainPartiallyExecutableBatch(t *testing.T) {
	f := newImportFixture(t, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/openai/refresh-token") {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		f.remote.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	if err := f.private.ConfigureTarget(context.Background(), server.URL, "test-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.Preview(context.Background(), "owner", accountworkbench.PreviewInput{Content: `["rt_invalid",` + importedJSON + `]`})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Errors) != 1 || preview.ID != "" || len(preview.Items) != 0 {
		t.Fatal("partially failed batch returned an executable preview")
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("failed preview created an import task")
	}
	if f.remote.created != 0 || f.remote.updates != 0 {
		t.Fatal("failed preview mutated an online account")
	}
}
