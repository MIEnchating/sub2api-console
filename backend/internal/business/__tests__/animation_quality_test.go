package business_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractQualityAnnotationsPersistWithVersionAndPrivateAudit(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if store != nil {
					_ = store.Close()
				}
			}()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			defer func() {
				for _, id := range []string{q.Hash("quality-contract"), q.Hash("second-quality-contract")} {
					if _, err := db.ExecContext(t.Context(), `DELETE FROM app_state WHERE "key"=?`, "animation-quality-sample/"+id); err != nil {
						t.Error(err)
					}
					if _, err := db.ExecContext(t.Context(), `DELETE FROM operation_audit WHERE object_type='animation-quality-sample' AND object_id=?`, id); err != nil {
						t.Error(err)
					}
				}
			}()
			sample := q.Sample{ID: q.Hash("quality-contract"), TaskID: "deleted-task", RequestID: "request-1", HTML: "private-source-marker", Prompt: "private-prompt-marker", SourceSHA256: q.Hash("private-source-marker"), PromptSHA256: q.Hash("private-prompt-marker"), Prediction: q.Prediction{Status: "unknown", Reason: "model_unavailable"}, Annotation: q.Annotation{Label: "normal", Reason: "private-reason-marker", SourceGroup: "template-a", UpdatedBy: "operator"}}
			saved, err := store.SaveAnimationQualitySample(t.Context(), sample, 0)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Annotation.Version != 1 || saved.CreatedAt == "" {
				t.Fatalf("%+v", saved)
			}
			annotations, err := store.AnimationQualityAnnotations(t.Context(), []string{sample.ID, q.Hash("missing")})
			if err != nil || len(annotations) != 1 || annotations[sample.ID].Annotation.Label != "normal" || annotations[sample.ID].SourceSHA256 != sample.SourceSHA256 || annotations[sample.ID].PromptSHA256 != sample.PromptSHA256 {
				t.Fatalf("annotation projection: %+v %v", annotations, err)
			}
			var wg sync.WaitGroup
			out := make(chan error, 2)
			for range 2 {
				wg.Go(func() { _, err := store.SaveAnimationQualitySample(t.Context(), sample, 1); out <- err })
			}
			wg.Wait()
			close(out)
			successes, conflicts := 0, 0
			for err := range out {
				if err == nil {
					successes++
				} else if errors.Is(err, business.ErrQualityAnnotationConflict) {
					conflicts++
				} else {
					t.Fatal(err)
				}
			}
			if successes != 1 || conflicts != 1 {
				t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
			}
			audits, err := store.AuditEvents(t.Context(), nil, false)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(audits)
			if strings.Contains(string(raw), "private-") {
				t.Fatal("audit leaked source, prompt or reason")
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := store.AnimationQualitySample(t.Context(), sample.ID)
			if err != nil || loaded == nil || loaded.HTML != sample.HTML || loaded.Prompt != sample.Prompt || loaded.Annotation.Version != 2 {
				t.Fatalf("persistence: %+v %v", loaded, err)
			}
			second := sample
			second.ID = q.Hash("second-quality-contract")
			if _, err = store.SaveAnimationQualitySample(t.Context(), second, 0); err != nil {
				t.Fatal(err)
			}
			page, err := store.AnimationQualitySamples(t.Context(), "", 1)
			if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
				t.Fatalf("page: %+v %v", page, err)
			}
			next, err := store.AnimationQualitySamples(t.Context(), page.NextCursor, 1)
			if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.NextCursor != "" {
				t.Fatalf("next: %+v %v", next, err)
			}
		})
	}
}
