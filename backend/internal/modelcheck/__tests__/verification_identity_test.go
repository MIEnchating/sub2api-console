package modelcheck_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestVerificationIdentityFollowsSourceAndReferenceEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, returnedModel          string
		officialStatus, targetTokens int
		wantTarget, wantOfficial     string
		mode                         string
	}{
		{"independent official account", "new-model", 200, 50, "observed", "official_source", ""},
		{"token difference is not identity evidence", "new-model", 200, 60, "observed", "official_source", ""},
		{"model alias", "provider-alias", 200, 50, "evidence_mismatch", "official_source", ""},
		{"reference unauthorized", "new-model", 401, 50, "observed", "inconclusive", ""},
		{"missing usage", "new-model", 200, 50, "observed", "official_source", "missing_usage"},
		{"missing returned model", "", 200, 50, "observed", "official_source", ""},
		{"partial reference", "new-model", 200, 50, "observed", "official_source", "partial"},
		{"reference alias", "new-model", 200, 50, "observed", "official_source", "reference_alias"},
		{"target unauthorized", "new-model", 200, 50, "inconclusive", "official_source", "target_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var referenceCalls atomic.Int32
			f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
				model, tokens := tc.returnedModel, tc.targetTokens
				if r.Header.Get("X-Test-Reference") == "yes" {
					if tc.officialStatus != 200 {
						w.WriteHeader(tc.officialStatus)
						return
					}
					if tc.mode == "partial" && referenceCalls.Add(1) > 1 {
						w.WriteHeader(429)
						return
					}
					model, tokens = "new-model", 50
					if tc.mode == "reference_alias" {
						model = "official-alias"
					}
				} else if tc.mode == "target_error" {
					w.WriteHeader(401)
					return
				}
				response := map[string]any{"output_text": "21", "model": model}
				if tc.mode != "missing_usage" {
					response["usage"] = map[string]any{"input_tokens": tokens, "output_tokens": 1}
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			local, _ := url.Parse(*f.catalog.details["2"].BaseURL)
			official := "https://api.openai.com"
			f.catalog.details["2"].BaseURL = &official
			previous := http.DefaultTransport
			http.DefaultTransport = oauthTransportFunc(func(r *http.Request) (*http.Response, error) {
				clone := r.Clone(r.Context())
				endpoint := *r.URL
				clone.URL = &endpoint
				if endpoint.Host == "api.openai.com" {
					clone.URL.Scheme, clone.URL.Host = local.Scheme, local.Host
					clone.Header.Set("X-Test-Reference", "yes")
				}
				return previous.RoundTrip(clone)
			})
			t.Cleanup(func() { http.DefaultTransport = previous })
			_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1", "2"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
			if err != nil {
				t.Fatal(err)
			}
			rows := finished(t, f).Result["tests"].([]map[string]any)
			for _, row := range rows {
				want := tc.wantTarget
				if row["account_id"] == "2" {
					want = tc.wantOfficial
				}
				authenticity := row["authenticity"].(map[string]any)
				if authenticity["status"] != want || authenticity["summary"] == "" {
					t.Fatalf("account %v: want %s with reason, got %v", row["account_id"], want, authenticity)
				}
			}
		})
	}
}

func TestVerificationNoReferenceExplainsMissingEvidence(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "21", "model": "new-model"})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	identity := row["authenticity"].(map[string]any)
	if identity["status"] != "observed" || strings.Contains(identity["summary"].(string), "官方对照") {
		t.Fatalf("missing reference must be distinguished from model failure: %v", row["authenticity"])
	}
}

func TestVerificationAliasWithoutReferenceDoesNotInventComparison(t *testing.T) {
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "21", "model": "provider-alias"})
	})
	_, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"new-model"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	row := finished(t, f).Result["tests"].([]map[string]any)[0]
	raw, _ := json.Marshal(row)
	var report modelcheck.VerificationReport
	_ = json.Unmarshal(raw, &report)
	if row["reference"] != nil || report.Authenticity.Status != "evidence_mismatch" {
		t.Fatalf("model alias is not an official comparison: %s", raw)
	}
}
