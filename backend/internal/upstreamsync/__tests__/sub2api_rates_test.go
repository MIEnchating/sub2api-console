package upstreamsync_test

import (
	"database/sql"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestSub2APICatalogUsesAuthenticatedUserRateByStableGroupID(t *testing.T) {
	for _, scenario := range []struct{ name, payload, want string }{
		{"exclusive replaces base", `{"code":0,"data":{"7":0.12,"99":0.01}}`, "0.12"},
		{"zero override", `{"code":0,"data":{"7":0}}`, "0"},
		{"no overrides", `{"code":0,"data":{}}`, "0.15"},
		{"null means no overrides", `{"code":0,"data":null}`, "0.15"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			reader, record := sub2APIRatesReader(t, http.StatusOK, scenario.payload)
			catalog, err := reader.ReadCatalog(t.Context(), record)
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Groups) != 2 || catalog.Groups[0].RawRate == nil || *catalog.Groups[0].RawRate != scenario.want {
				t.Fatalf("user rate not resolved: %+v", catalog.Groups)
			}
			if catalog.Groups[1].RawRate == nil || *catalog.Groups[1].RawRate != "0.3" {
				t.Fatal("same-name group inherited another group's rate")
			}
		})
	}
}

func TestSub2APICatalogRejectsFailedOrMalformedExclusiveRateRead(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		status  int
		payload string
	}{
		{"unauthorized", 401, `{}`}, {"forbidden", 403, `{}`}, {"server error", 500, `{}`},
		{"business error", 200, `{"code":1,"message":"rates unavailable"}`},
		{"array", 200, `{"code":0,"data":[]}`},
		{"missing data", 200, `{"code":0}`},
		{"negative", 200, `{"code":0,"data":{"7":-0.12}}`},
		{"invalid decimal", 200, `{"code":0,"data":{"7":"1/3"}}`},
		{"null rate", 200, `{"code":0,"data":{"7":null}}`},
		{"name instead of ID", 200, `{"code":0,"data":{"same-name":0.12}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			reader, record := sub2APIRatesReader(t, scenario.status, scenario.payload)
			catalog, err := reader.ReadCatalog(t.Context(), record)
			if err == nil || len(catalog.Groups) > 0 {
				t.Fatalf("failed override read must not return base-rate catalog: %+v %v", catalog, err)
			}
		})
	}
}

func TestSub2APICatalogSupportsOldServerWithoutUserRatesEndpoint(t *testing.T) {
	reader, record := sub2APIRatesReader(t, 404, `{}`)
	catalog, err := reader.ReadCatalog(t.Context(), record)
	if err != nil || len(catalog.Groups) != 2 || catalog.Groups[0].RawRate == nil || *catalog.Groups[0].RawRate != "0.15" {
		t.Fatalf("old endpoint fallback: %+v %v", catalog, err)
	}
}

func sub2APIRatesReader(t *testing.T, status int, payload string) (*upstreamsync.Reader, configstore.AuthRecord) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":7,"name":"same-name","rate_multiplier":0.15},{"id":8,"name":"same-name","rate_multiplier":0.3}]}`))
		case "/api/v1/groups/rates":
			w.WriteHeader(status)
			_, _ = w.Write([]byte(payload))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":41,"name":"key","group_id":7}],"total":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	token := "fixture-token"
	return upstreamsync.NewReader(server.Client()), configstore.AuthRecord{BaseURL: server.URL, UpstreamType: "sub2api", AccessToken: &token}
}

func TestExclusiveRateSyncUpdatesDisplayedCostAndPreservesItWhenNextReadFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "rates.example.test", BaseURL: "https://rates.example.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "2"}); err != nil {
		t.Fatal(err)
	}
	private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	for _, failed := range []bool{false, true} {
		status, payload := 200, `{"code":0,"data":{"7":0.12}}`
		if failed {
			status, payload = 500, `{}`
		}
		reader, record := sub2APIRatesReader(t, status, payload)
		record.Host, record.AuthMode = "rates.example.test", "sub2api_user_token"
		if err := private.SaveAuthRecord(t.Context(), record, nil); err != nil {
			t.Fatal(err)
		}
		result, err := upstreamsync.New(store, private, reader, nil, nil).SyncHost(t.Context(), record.Host, upstreamsync.Scope{Catalog: true}, "test")
		if !failed && (err != nil || result.Status != "succeeded") {
			t.Fatalf("sync: %+v %v", result, err)
		}
		if failed && err == nil && result.Status == "succeeded" {
			t.Fatal("failed exclusive rate read reported success")
		}
		var raw, effective string
		if err := db.QueryRow(`SELECT raw_rate,effective_rate FROM upstream_groups WHERE host='rates.example.test' AND group_id='7'`).Scan(&raw, &effective); err != nil {
			t.Fatal(err)
		}
		if raw != "0.12" || effective != "0.06" {
			t.Fatalf("exclusive rate must be mapped exactly once and retained on failure: raw=%s effective=%s", raw, effective)
		}
	}
}
