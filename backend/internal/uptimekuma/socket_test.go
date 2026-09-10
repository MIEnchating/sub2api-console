package uptimekuma

import (
	"context"
	"testing"
)

func TestLoginAcceptsParameterlessLifecycleEventsBeforeAcknowledgement(t *testing.T) {
	f := newFixture(t)
	f.initialEvents = []string{`42["loginRequired"]`}
	f.loginEvents = []string{`42["initServerTimezone"]`}
	c := f.configure(t, true)
	if !c.ManagementConfigured {
		t.Fatal("successful login did not enable management")
	}
}

func TestLoginAcceptsStringHeartbeatIDsAndKeepsTheirStatus(t *testing.T) {
	f := newFixture(t)
	f.loginEvents = []string{`42["heartbeatList","19",[{"status":0},{"status":1}],true]`}
	f.configure(t, true)
	f.mu.Lock()
	f.rejectMetrics = true
	f.mu.Unlock()
	snapshot, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Monitors) != 1 || snapshot.Monitors[0].ID != 19 || snapshot.Monitors[0].Status == nil || *snapshot.Monitors[0].Status != 1 {
		t.Fatalf("heartbeat status was lost: %#v", snapshot.Monitors)
	}
}

func TestSocketRejectsMalformedKnownEventsAndNonIDHeartbeatKeys(t *testing.T) {
	for _, packet := range []string{`42[]`, `42[false]`, `42["monitorList"]`, `42["heartbeatList","monitor-name",[]]`, `42["heartbeatList",0,[]]`, `42["heartbeatList","019",[]]`} {
		t.Run(packet, func(t *testing.T) {
			s := &socket{statuses: map[int64]int{}}
			if s.event(packet) == nil {
				t.Fatal("malformed event accepted")
			}
		})
	}
}
