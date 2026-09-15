package onboarding_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

type probeRepository struct{ onboarding.Repository }

func (probeRepository) OnboardingCandidates(context.Context, string) ([]business.OnboardingCandidate, error) {
	group, key := "6", "91"
	return []business.OnboardingCandidate{{GroupID: &group, UpstreamKeyID: &key}}, nil
}

type probePrivate struct {
	onboarding.PrivateStore
	baseURL string
}

func (store probePrivate) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return &configstore.AuthRecord{Host: "upstream.test", BaseURL: store.baseURL}, nil
}

func TestProbeRejectsFailedOrMalformedEventAfterStreamingText(t *testing.T) {
	for _, test := range []struct {
		name, event string
	}{
		{name: "explicit failure", event: `{"type":"error","error":{"message":"generation failed"}}`},
		{name: "response failure", event: `{"type":"response.failed","response":{"status":"failed","error":{"message":"generation failed"}}}`},
		{name: "malformed event", event: `{"type":"response.completed"} trailing`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = writer.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: " + test.event + "\n\n"))
			}))
			defer server.Close()
			service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)

			result, err := service.Probe(context.Background(), "upstream.test", "6", "gpt-test", "stream")

			if err == nil || result.Status != "failed" {
				t.Fatalf("invalid final stream was accepted: result=%+v error=%v", result, err)
			}
			if test.name != "malformed event" && !strings.Contains(err.Error(), "generation failed") {
				t.Fatalf("stream failure lost its cause: %v", err)
			}
		})
	}
}
