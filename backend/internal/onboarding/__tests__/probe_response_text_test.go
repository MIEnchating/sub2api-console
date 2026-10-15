package onboarding_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
)

func TestProbeReturnsAllTextChunksWithoutRepeatingCompletedSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"你好，\"}\n\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"连接正常。\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output_text\":\"你好，连接正常。\"}}\n\n")
	}))
	defer server.Close()
	service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)
	result, err := service.Probe(context.Background(), "upstream.test", "6", "gpt-test", "stream")
	if err != nil || result.ResponseText != "你好，连接正常。" {
		t.Fatalf("response lost or duplicated content: %+v err=%v", result, err)
	}
}

func TestProbePublishesCompletedStreamWithoutWaitingForConnectionClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"完成\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	service := onboarding.New(probeRepository{}, probePrivate{baseURL: server.URL}, &keyClient{}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := service.Probe(ctx, "upstream.test", "6", "gpt-test", "stream")
	if err != nil || result.Status != "passed" || result.ResponseText != "完成" {
		t.Fatalf("completed stream was not returned: %+v err=%v", result, err)
	}
}
