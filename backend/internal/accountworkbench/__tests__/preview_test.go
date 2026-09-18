package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func previewOwner(t *testing.T, store *configstore.Store) (string, string) {
	t.Helper()
	token, err := store.CreateSession(context.Background(), "test-admin", time.Hour, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:]), token
}
func TestPreviewStoresSecretsPrivatelyAndReplacesPreviousSessionPreview(t *testing.T) {
	service, store := fixture(t, `{"data":`+sourceAccount+`}`)
	ctx := context.Background()
	owner, _ := previewOwner(t, store)
	input := accountworkbench.PreviewInput{Action: "import", Content: "rt_private-secret", Check: true, Promote: true}
	first, err := service.Preview(ctx, owner, input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "private-secret") || strings.Contains(string(raw), "test-admin") {
		t.Fatal("public preview exposed private data")
	}
	private, version, err := store.WorkbenchDocument(ctx, "preview:"+owner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(private), "rt_private-secret") || version != first.Revision {
		t.Fatal("preview did not preserve private execution data")
	}
	input.Content = "rt_replacement"
	second, err := service.Preview(ctx, owner, input)
	if err != nil {
		t.Fatal(err)
	}
	private, _, _ = store.WorkbenchDocument(ctx, "preview:"+owner)
	if second.ID == first.ID || second.Revision != first.Revision+1 || strings.Contains(string(private), "private-secret") {
		t.Fatal("session preview not replaced")
	}
}
func TestPreviewRejectsRevokedOwnerBeforeStoringCredentials(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, token := previewOwner(t, store)
	ctx := context.Background()
	if err := store.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_test"}); err == nil {
		t.Fatal("revoked owner accepted")
	}
	raw, _, err := store.WorkbenchDocument(ctx, "preview:"+owner)
	if err != nil || len(raw) > 0 {
		t.Fatal("revoked session stored preview")
	}
}
func TestPreviewExportDoesNotReadManagementTargetOrDeduplicate(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	ctx := context.Background()
	// Explicit local scope must never create a management request.
	service.UseTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("export contacted management target")
		return nil, nil
	}))
	preview, err := service.Preview(ctx, owner, accountworkbench.PreviewInput{Action: "export", Content: "rt_same\nrt_same", Check: true, Promote: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || preview.Check || preview.Promote || preview.Template != nil {
		t.Fatal("local export inherited managed behavior")
	}
	private, _, _ := store.WorkbenchDocument(ctx, "preview:"+owner)
	if !strings.Contains(string(private), `"scope":"local-export"`) {
		t.Fatal("independent export lacks explicit scope")
	}
	if strings.Contains(string(private), "isolated.invalid") || strings.Contains(string(private), "test-admin") {
		t.Fatal("local export retained management credentials")
	}
}
func TestPreviewDisabledProxyIgnoresRememberedAddress(t *testing.T) {
	service, store := fixture(t, `{}`)
	owner, _ := previewOwner(t, store)
	input := accountworkbench.PreviewInput{Action: "export", Content: "rt_test", ProxyURL: "invalid-old-proxy"}
	if _, err := service.Preview(context.Background(), owner, input); err != nil {
		t.Fatal(err)
	}
	input.ProxyEnabled = true
	if _, err := service.Preview(context.Background(), owner, input); err == nil {
		t.Fatal("enabled invalid proxy accepted")
	}
}
