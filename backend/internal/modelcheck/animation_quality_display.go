package modelcheck

import (
	"context"
	"encoding/json"
	q "github.com/MIEnchating/sub2api-console/backend/internal/animationquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Overlay the latest review for display without modifying running generation
// snapshots, the original automatic prediction, or human training samples.
func (s *Service) AnimationQualityDisplay(ctx context.Context, task taskstore.Task) (taskstore.Task, error) {
	animation := task.Skill == animationSkill && (task.Operation == "account-model-animation" || task.Operation == "account-model-combined" || task.Operation == "managed-model-detection")
	onboarding := task.Skill == "sub2api-account-onboarding" && task.Operation == "onboarding-probe-probe"
	if !animation && !onboarding {
		return task, nil
	}
	repo, ok := s.accounts.(qualityDisplayRepository)
	if !ok {
		return task, nil
	}
	raw, err := json.Marshal(task.Result)
	if err != nil {
		return task, err
	}
	var result map[string]any
	if err = json.Unmarshal(raw, &result); err != nil {
		return task, err
	}
	rows, _ := result["animations"].([]any)
	if probe, ok := result["probe_result"].(map[string]any); ok && probe["animation"] != nil {
		rows = append(rows, probe["animation"])
	}
	targets := make(map[string]map[string]any)
	for _, rawRow := range rows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		account, _ := row["account_id"].(string)
		request, _ := row["request_id"].(string)
		if request == "" || row["status"] != "succeeded" {
			continue
		}
		targets[q.Hash(task.ID+"\x00"+account+"\x00"+request)] = row
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	states, err := repo.AnimationQualityRechecks(ctx, ids)
	if err != nil {
		return task, err
	}
	annotations, err := repo.AnimationQualityAnnotations(ctx, ids)
	if err != nil {
		return task, err
	}
	for id, row := range targets {
		source, _ := row["source"].(string)
		prompt, _ := row["prompt"].(string)
		sourceHash, promptHash := q.Hash(source), q.Hash(prompt)
		if state, ok := states[id]; ok && state.Prediction != nil && state.SourceSHA256 == sourceHash && state.PromptSHA256 == promptHash {
			row["quality"] = state.Prediction
		}
		if rawPrediction, marshalErr := json.Marshal(row["quality"]); marshalErr == nil && row["quality"] != nil {
			var prediction q.Prediction
			if json.Unmarshal(rawPrediction, &prediction) == nil {
				row["quality_prediction_sha256"] = q.ReviewFingerprint(q.Sample{SourceSHA256: sourceHash, PromptSHA256: promptHash, Prediction: prediction})
			}
		}
		if record, ok := annotations[id]; ok && record.SourceSHA256 == sourceHash && record.PromptSHA256 == promptHash {
			row["quality_annotation"] = record.Annotation
		}
	}

	task.Result = result
	return task, nil
}
