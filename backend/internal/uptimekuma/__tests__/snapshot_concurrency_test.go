package uptimekuma_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/coder/websocket"
)

func TestSnapshotStartsManagementBeforeMetricsCompletesAndMergesByStableID(t *testing.T) {
	listStarted := make(chan struct{})
	peer := &snapshotPeer{
		metrics: func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-listStarted:
				_, _ = w.Write([]byte(snapshotMetrics))
			case <-r.Context().Done():
			}
		},
		list: func(context.Context, *websocket.Conn) bool {
			close(listStarted)
			return true
		},
	}
	service, _, _ := newSnapshotFixture(t, peer, true)
	result, err := service.Snapshot(snapshotTestContext(t))
	if err != nil || result.Warning != "" || len(result.Monitors) != 2 {
		t.Fatalf("independent reads did not complete together: %v %+v", err, result)
	}
	first, second := result.Monitors[0], result.Monitors[1]
	if first.ID != 19 || first.Status == nil || *first.Status != 1 ||
		first.ResponseTime == nil || *first.ResponseTime != 42 ||
		first.TemplateID != "isolated-template" || first.TemplateRevision != 3 {
		t.Fatalf("stable monitor enrichment lost: %+v", first)
	}
	if second.ID != 23 || second.Status == nil || *second.Status != 0 || second.ResponseTime != nil {
		t.Fatalf("name-only or other-ID metrics overwrote monitor 23: %+v", second)
	}
	body, err := json.Marshal(result)
	if err != nil || strings.Contains(string(body), "private-secret") || strings.Contains(string(body), "isolated-key") {
		t.Fatal("snapshot exposed private configuration")
	}
	if peer.metricsCalls.Load() != 1 || peer.socketCalls.Load() != 1 || peer.listCalls.Load() != 1 {
		t.Fatal("snapshot replayed a read")
	}
}

func TestSnapshotManagementFailureCancelsAndJoinsPendingMetrics(t *testing.T) {
	peer := &snapshotPeer{
		metrics: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
	}
	peer.login = func(ctx context.Context) bool {
		select {
		case <-peer.metricsStart:
		case <-ctx.Done():
		}
		return false
	}
	service, _, revision := newSnapshotFixture(t, peer, true)
	result, err := service.Snapshot(snapshotTestContext(t))
	if err == nil || uptimekuma.PublicError(err).Code != "kuma_auth_failed" || len(result.Monitors) != 0 {
		t.Fatalf("management failure was hidden: %v %+v", err, result)
	}
	select {
	case stopped := <-peer.metricsFinish:
		if !errors.Is(stopped, context.Canceled) {
			t.Fatalf("metrics waited for its timeout instead of sibling cancellation: %v", stopped)
		}
	default:
		t.Fatal("snapshot returned with metrics still running")
	}
	if err := service.Disconnect(snapshotTestContext(t), revision); err != nil {
		t.Fatalf("failed snapshot kept the service gate: %v", err)
	}
}

func TestSnapshotCallerCancellationStopsBothReadsAndReleasesService(t *testing.T) {
	listStarted := make(chan struct{})
	peer := &snapshotPeer{
		metrics: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
		list: func(ctx context.Context, conn *websocket.Conn) bool {
			close(listStarted)
			_, _, _ = conn.Read(ctx)
			return false
		},
	}
	service, _, revision := newSnapshotFixture(t, peer, true)
	ctx, cancel := context.WithCancel(snapshotTestContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := service.Snapshot(ctx)
		done <- err
	}()
	waitSnapshotEvent(t, ctx, peer.metricsStart)
	waitSnapshotEvent(t, ctx, listStarted)
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled snapshot returned success")
		}
	case <-snapshotTestContext(t).Done():
		t.Fatal("cancelled snapshot did not finish")
	}
	waitSnapshotEvent(t, snapshotTestContext(t), peer.socketStopped)
	select {
	case stopped := <-peer.metricsFinish:
		if !errors.Is(stopped, context.Canceled) {
			t.Fatalf("metrics did not share caller cancellation: %v", stopped)
		}
	default:
		t.Fatal("metrics remained active after cancellation returned")
	}
	if err := service.Disconnect(snapshotTestContext(t), revision); err != nil {
		t.Fatalf("cancelled snapshot kept the service gate: %v", err)
	}
}

func TestSnapshotMetricsFailureKeepsManagementListAndTemplateMetadata(t *testing.T) {
	peer := &snapshotPeer{metrics: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}}
	service, _, _ := newSnapshotFixture(t, peer, true)
	result, err := service.Snapshot(snapshotTestContext(t))
	if err != nil || result.Warning == "" || len(result.Monitors) != 2 {
		t.Fatalf("metrics failure did not retain management data: %v %+v", err, result)
	}
	if result.Monitors[0].Status == nil || *result.Monitors[0].Status != 0 ||
		result.Monitors[0].TemplateID != "isolated-template" {
		t.Fatalf("management fallback lost heartbeat/template data: %+v", result.Monitors[0])
	}
}

func TestSnapshotMetricsOnlyNeverOpensManagementConnection(t *testing.T) {
	peer := &snapshotPeer{}
	service, _, _ := newSnapshotFixture(t, peer, false)
	result, err := service.Snapshot(snapshotTestContext(t))
	if err != nil || result.Warning != "" || len(result.Monitors) != 3 {
		t.Fatalf("metrics-only result changed: %v %+v", err, result)
	}
	if peer.metricsCalls.Load() != 1 || peer.socketCalls.Load() != 0 {
		t.Fatal("metrics-only configuration attempted management access")
	}
}

func TestSnapshotCancellationAfterManagementCompletesDoesNotReturnSuccessfulFallback(t *testing.T) {
	peer := &snapshotPeer{
		metrics: func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
	}
	service, _, _ := newSnapshotFixture(t, peer, true)
	ctx, cancel := context.WithCancel(snapshotTestContext(t))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := service.Snapshot(ctx)
		done <- err
	}()
	waitSnapshotEvent(t, ctx, peer.metricsStart)
	// The management connection closes as soon as its part completes, while
	// metrics remains pending. Cancellation must not become a successful warning.
	waitSnapshotEvent(t, ctx, peer.socketStopped)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled caller received a successful fallback: %v", err)
		}
	case <-snapshotTestContext(t).Done():
		t.Fatal("snapshot did not stop waiting for metrics")
	}
}

func TestSnapshotManagementFailureClosesPendingMetricsBodyBeforeReturning(t *testing.T) {
	peer := &snapshotPeer{
		metrics: func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("# TYPE monitor_status gauge\n"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		},
	}
	peer.login = func(ctx context.Context) bool {
		// Server Flush does not prove the client received the response body.
		// Fail management only once metrics has actually started reading it.
		select {
		case <-peer.metricsRead:
		case <-ctx.Done():
		}
		return false
	}
	service, _, _ := newSnapshotFixture(t, peer, true)
	_, err := service.Snapshot(snapshotTestContext(t))
	if err == nil || uptimekuma.PublicError(err).Code != "kuma_auth_failed" {
		t.Fatalf("body cancellation hid management failure: %v", err)
	}
	select {
	case <-peer.metricsClosed:
	default:
		t.Fatal("snapshot returned before closing metrics response body")
	}
}
