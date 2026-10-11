package uptimekuma_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestBusyServicePreservesCallerCancellationWithoutInterruptingActiveRead(t *testing.T) {
	finish := make(chan struct{})
	peer := &snapshotPeer{metrics: func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-finish:
			_, _ = w.Write([]byte(snapshotMetrics))
		case <-r.Context().Done():
		}
	}}
	service, _, revision := newSnapshotFixture(t, peer, false)
	active, stop := context.WithCancel(snapshotTestContext(t))
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := service.Snapshot(active); done <- err }()
	waitSnapshotEvent(t, active, peer.metricsStart)

	for _, call := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"snapshot", func(ctx context.Context) error { _, err := service.Snapshot(ctx); return err }},
		{"resources", func(ctx context.Context) error { _, err := service.Resources(ctx, "status-pages"); return err }},
		{"disconnect", func(ctx context.Context) error { return service.Disconnect(ctx, revision) }},
		{"save", func(ctx context.Context) error {
			_, err := service.Save(ctx, uptimekuma.ConfigInput{Revision: revision})
			return err
		}},
	} {
		for _, expired := range []bool{false, true} {
			name := "cancelled"
			want := context.Canceled
			if expired {
				name, want = "deadline", context.DeadlineExceeded
			}
			t.Run(call.name+"/"+name, func(t *testing.T) {
				var ctx context.Context
				var cancel context.CancelFunc
				if expired {
					ctx, cancel = context.WithDeadline(t.Context(), time.Unix(1, 0))
				} else {
					ctx, cancel = context.WithCancel(t.Context())
					cancel()
				}
				defer cancel()
				if err := call.run(ctx); !errors.Is(err, want) {
					t.Fatalf("caller error = %v, want %v", err, want)
				}
			})
		}
	}
	close(finish)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("active read interrupted: %v", err)
		}
	case <-active.Done():
		t.Fatal("active read did not finish")
	}
	if peer.metricsCalls.Load() != 1 || peer.socketCalls.Load() != 0 {
		t.Fatal("cancelled request reached remote")
	}
	if err := service.Disconnect(t.Context(), revision); err != nil {
		t.Fatalf("gate not released: %v", err)
	}
}
