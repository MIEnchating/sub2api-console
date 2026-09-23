package newapimanagement_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func westernCatalog(t *testing.T, fail *bool) *newapimanagement.Service {
	t.Helper()
	page := officialPage
	return setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
		var file, body string
		switch r.URL.Host {
		case "raw.githubusercontent.com":
			body = `{"grok-4.7":{"input_cost_per_token":9,"output_cost_per_token":9},"gpt-5.4":{"input_cost_per_token":9,"output_cost_per_token":9},"claude-opus-5-5":{"input_cost_per_token":9,"output_cost_per_token":9},"other-model":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000004}}`
		case "newapi.test":
			body = `{"success":true,"data":[]}`
			if r.URL.Path == "/api/channel/models_enabled" {
				body = `{"success":true,"data":["grok-4.7","claude-opus-5-5","gpt-5.4","gpt-5.4-2099-01-01"]}`
			}
		case "docs.x.ai":
			file = "grok.md"
		case "developers.openai.com":
			file = "openai.md"
			if strings.HasPrefix(r.URL.Path, "/api/docs/models/") {
				file = "model-details/" + path.Base(r.URL.Path)
			}
		case "platform.claude.com":
			file = "claude.md"
			if strings.HasPrefix(r.URL.Path, "/docs/en/models/") {
				file = "model-details/claude-" + path.Base(path.Dir(r.URL.Path)) + ".md"
			}
		default:
			return nil, fmt.Errorf("test refuses endpoint %s; official models must not use the admin fallback", r.URL)
		}
		if file != "" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
				t.Error("official source received credentials")
			}
			raw, err := os.ReadFile(filepath.Join("..", "..", "officialpricing", "__tests__", "testdata", file))
			if err != nil {
				return nil, err
			}
			body = string(raw)
			if *fail {
				body = "<html>maintenance</html>"
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
}

func TestWesternOfficialCatalogOverridesRemotePricesAndRetainsCacheOnFailure(t *testing.T) {
	fail := false
	service := westernCatalog(t, &fail)
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Stale {
		t.Fatalf("official refresh failed: %s", catalog.Warning)
	}
	for model, input := range map[string]string{"grok-4.7": "0.000002", "claude-opus-5-5": "0.000004", "gpt-5.4": "0.0000025"} {
		p := priceByName(t, catalog, model)
		if p.Source != "official" || p.InputPrice != input || p.SourceURL == "" || p.BillingExpr == "" {
			t.Fatalf("official price not authoritative: %+v", p)
		}
	}
	claude := priceByName(t, catalog, "claude-opus-5-5")
	if claude.CacheWrite1hPrice != "0.000008" || claude.CreateCache1hRatio != "2" {
		t.Fatalf("one-hour cache price lost: %+v", claude)
	}
	if len(catalog.MissingModels) != 1 || catalog.MissingModels[0] != "gpt-5.4-2099-01-01" {
		t.Fatalf("unpublished snapshot was inferred: %v", catalog.MissingModels)
	}
	image := priceByName(t, catalog, "grok-imagine-image-2.0")
	if image.Mode != "image_generation" || image.SyncError == "" || image.ModelRatio != "" {
		t.Fatalf("per-image price became token billing: %+v", image)
	}
	fail = true
	stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale {
		t.Fatal("failed official refresh was reported as fresh")
	}
	for _, model := range []string{"grok-4.7", "claude-opus-5-5", "gpt-5.4"} {
		p := priceByName(t, stale, model)
		if p.Source != "official" || p.BillingExpr != priceByName(t, catalog, model).BillingExpr {
			t.Fatalf("official cache replaced on failure: %+v", p)
		}
	}
}

func TestWesternOfficialFirstFailureDoesNotUseThirdPartyOrDefaultPrices(t *testing.T) {
	fail := true
	catalog, err := westernCatalog(t, &fail).ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Stale || len(catalog.MissingModels) != 4 {
		t.Fatalf("missing official prices not reported: %+v", catalog)
	}
	if len(catalog.Models) != 1 || catalog.Models[0].Model != "other-model" {
		t.Fatal("unverified third-party prices presented as official")
	}
}
