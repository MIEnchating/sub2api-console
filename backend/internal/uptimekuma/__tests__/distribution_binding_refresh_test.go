package uptimekuma_test

import (
	"encoding/json"
	"testing"
)

func TestTemplateDistributionRechecksBindingBeforeEachWrite(t *testing.T) {
	f, peer, template := distributionFixture(t)
	preview := distributionPreview(t, f, template)
	peer.afterWrite = func(id int64) {
		if id == 1 {
			if err := f.config.DeleteKumaMonitorTemplate(f.ctx, f.server.URL, 2); err != nil {
				t.Errorf("delete next binding: %v", err)
			}
		}
	}
	runDistribution(t, f, preview)
	final := f.completed(t)
	if final.Status != "failed" || final.Result["completed"] != json.Number("1") {
		t.Fatalf("changed binding was accepted: %+v", final)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 1 || peer.writes[0] != 1 {
		t.Fatalf("unexpected writes: %v", peer.writes)
	}
}
