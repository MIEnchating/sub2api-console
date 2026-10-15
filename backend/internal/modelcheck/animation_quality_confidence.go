package modelcheck

import (
	"context"
	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type qualityDisplayRepository interface {
	AnimationQualityRechecks(context.Context, []string) (map[string]q.Recheck, error)
	AnimationQualityAnnotations(context.Context, []string) (map[string]q.AnnotationRecord, error)
}

// Stable request identities bind each quality result to one historical generation.
// Reassessment changes its verdict, never its completion time or sample count.
func (s *Service) overlayQualityConfidence(ctx context.Context, rows []taskstore.DetectionConfidenceResult) error {
	repo, ok := s.accounts.(qualityDisplayRepository)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Animation && row.RequestID != "" {
			ids = append(ids, q.Hash(row.TaskID+"\x00"+row.AccountID+"\x00"+row.RequestID))
		}
	}
	rechecks, err := repo.AnimationQualityRechecks(ctx, ids)
	if err != nil {
		return err
	}
	annotations, err := repo.AnimationQualityAnnotations(ctx, ids)
	if err != nil {
		return err
	}
	for i := range rows {
		row := &rows[i]
		if !row.Animation || row.RequestID == "" {
			continue
		}
		id := q.Hash(row.TaskID + "\x00" + row.AccountID + "\x00" + row.RequestID)
		if state, exists := rechecks[id]; exists && state.Prediction != nil {
			row.Verdict = q.QualityVerdict(state.Prediction.Status)
		}
		if record, exists := annotations[id]; exists && record.Annotation.Source != "confirmed_auto" {
			row.Verdict = q.QualityVerdict(record.Annotation.Label)
		}
	}
	return nil
}
