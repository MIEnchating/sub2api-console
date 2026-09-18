package adminclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func TestDirectProbeResolvesModelMappingBeforeGeneration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mapping map[string]string
		model   string
		want    string
	}{
		{"catch_all", map[string]string{"*": "grok-4"}, "alias", "grok-4"},
		{"prefix", map[string]string{"grok-*": "grok-4"}, "grok-latest", "grok-4"},
		{"exact_before_wildcard", map[string]string{"*": "fallback", "grok-*": "family", "grok-4": "exact"}, "grok-4", "exact"},
		{"longest_prefix", map[string]string{"*": "fallback", "grok-*": "family", "grok-4*": "specific"}, "grok-4-fast", "specific"},
		{"unrelated_wildcard", map[string]string{"claude-*": "claude-sonnet"}, "grok-4", "grok-4"},
		{"unrelated_wildcard_target", map[string]string{"claude-*": "claude-*", "gpt-*": "gpt-*", "o3*": "o3*", "grok-4.5": "grok-4.5"}, "grok-4.5", "grok-4.5"},
		{"exact_overrides_wildcard_target", map[string]string{"grok-*": "grok-*", "grok-4.5": "grok-4.5"}, "grok-4.5", "grok-4.5"},
		{"specific_overrides_wildcard_target", map[string]string{"*": "*", "grok-*": "grok-4.5"}, "grok-latest", "grok-4.5"},
		{"unmatched_wildcard_target", map[string]string{"claude-*": "claude-*"}, "grok-4.5", "grok-4.5"},
		{"case_sensitive", map[string]string{"Grok-*": "other"}, "grok-4", "grok-4"},
		{"literal_context_suffix", map[string]string{"claude-sonnet[1m]": "claude-sonnet[1m]"}, "claude-sonnet[1m]", "claude-sonnet[1m]"},
		{"unrelated_literal_suffix", map[string]string{"claude-sonnet[1m]": "claude-sonnet[1m]", "grok-4": "grok-4"}, "grok-4", "grok-4"},
		{"single_mapping_pass", map[string]string{"grok-*": "alias", "alias": "other"}, "grok-4", "alias"},
		{"empty_mapping", map[string]string{}, "grok-4", "grok-4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != tc.want {
					t.Errorf("wire model = %q, want %q", body.Model, tc.want)
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			t.Cleanup(upstream.Close)
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": "grok",
				"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL, "model_mapping": tc.mapping},
			})
			response, err := client.OpenAccountProbe(context.Background(), "41", tc.model, "hi")
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if calls.Load() != 1 {
				t.Fatalf("generation calls = %d, want 1", calls.Load())
			}
		})
	}
}

func TestDirectProbeCatchAllRejectsInvalidRequestModelWithoutGeneration(t *testing.T) {
	for _, model := range []string{"", " ", "grok\x00-4"} {
		t.Run(model, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			t.Cleanup(upstream.Close)
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": "grok",
				"credentials": map[string]any{"api_key": "test-only-key", "base_url": upstream.URL, "model_mapping": map[string]string{"*": "grok-4"}},
			})
			response, err := client.OpenAccountProbe(context.Background(), "41", model, "hi")
			if response != nil {
				_ = response.Body.Close()
			}
			var unavailable *adminclient.ProbeUnavailableError
			if !errors.As(err, &unavailable) || unavailable.Code != "probe_model_invalid" || calls.Load() != 0 {
				t.Fatalf("invalid model must not be replaced by catch-all: error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestDirectProbeSelectedWildcardTargetSkipsWithoutGeneration(t *testing.T) {
	for _, mapping := range []map[string]string{
		{"grok-*": "grok-*"},
		{"grok-4.5": "*"},
	} {
		t.Run(fmt.Sprint(mapping), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			t.Cleanup(upstream.Close)
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": "grok",
				"credentials": map[string]any{"api_key": "test-only-key", "base_url": upstream.URL, "model_mapping": mapping},
			})
			probe, err := client.PrepareAccountProbe(context.Background(), "41")
			if err != nil {
				t.Fatal(err)
			}
			_, err = probe.PerformanceDescriptor("grok-4.5", "hi")
			var unavailable *adminclient.ProbeUnavailableError
			if !errors.As(err, &unavailable) || unavailable.Code != "probe_mapping_unsupported" {
				t.Fatalf("invalid workload must retain skip code: %v", err)
			}
			response, err := probe.Open(context.Background(), "grok-4.5", "hi")
			if response != nil {
				_ = response.Body.Close()
			}
			if !errors.As(err, &unavailable) || unavailable.Code != "probe_mapping_unsupported" || calls.Load() != 0 {
				t.Fatalf("wildcard must never reach generation: error=%v calls=%d", err, calls.Load())
			}
		})
	}
}
