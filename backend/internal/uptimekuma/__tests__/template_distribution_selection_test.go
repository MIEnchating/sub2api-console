package uptimekuma_test

import (
	"encoding/json"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"testing"
)

func TestDistributionOnlyWritesSelectedMonitor(t *testing.T) {
	f, peer, template := distributionFixture(t)
	preview := distributionPreview(t, f, template)
	input := uptimekuma.TemplateDistributionInput{Revision: preview.Revision, Scope: preview.Scope}
	if err := json.Unmarshal([]byte(`{"monitor_ids":[2]}`), &input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.DistributeTemplate(f.ctx, template.ID, input); err != nil {
		t.Fatal(err)
	}
	final := f.completed(t)
	if final.Status != "succeeded" || final.Result["total"] != json.Number("1") {
		t.Fatalf("wrong selected result: %#v", final)
	}
	peer.mu.Lock()
	defer peer.mu.Unlock()
	if len(peer.writes) != 1 || peer.writes[0] != 2 {
		t.Fatalf("unselected monitor written: %v", peer.writes)
	}
	links, err := f.config.KumaMonitorTemplates(f.ctx, f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if links[1].Revision != 1 || links[2].Revision != template.Revision {
		t.Fatal("unselected binding changed")
	}
}

func TestDistributionRejectsInvalidSelectionBeforeAnyWrite(t *testing.T) {
	for _, payload := range []string{`{}`, `{"monitor_ids":[]}`, `{"monitor_ids":null}`, `{"monitor_ids":[0]}`, `{"monitor_ids":[1,1]}`, `{"monitor_ids":[1,3]}`, `{"monitor_ids":[1,999]}`} {
		t.Run(payload, func(t *testing.T) {
			f, peer, template := distributionFixture(t)
			preview := distributionPreview(t, f, template)
			input := uptimekuma.TemplateDistributionInput{Revision: preview.Revision, Scope: preview.Scope}
			if err := json.Unmarshal([]byte(payload), &input); err != nil {
				t.Fatal(err)
			}
			_, err := f.service.DistributeTemplate(f.ctx, template.ID, input)
			if err == nil {
				final := f.completed(t)
				if final.Status != "failed" || final.Result["remote_write"] != false {
					t.Fatalf("invalid selection accepted: %#v", final)
				}
			}
			peer.mu.Lock()
			defer peer.mu.Unlock()
			if len(peer.writes) != 0 {
				t.Fatalf("invalid selection wrote: %v", peer.writes)
			}
		})
	}
}
