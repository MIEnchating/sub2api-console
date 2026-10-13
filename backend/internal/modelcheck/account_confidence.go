package modelcheck

import (
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func accountConfidence(tasks []taskstore.Task, now time.Time) map[string]accountquality.Statistics {
	result := map[string]accountquality.Statistics{}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			continue
		}
		for _, id := range taskAccountIDs(task) {
			if _, ok := result[id]; !ok {
				result[id] = accountquality.New(now)
			}
		}
		at, err := time.Parse(time.RFC3339Nano, task.UpdatedAt)
		if err != nil {
			continue
		}
		// Visit each check once, rather than allocating and scanning all rows
		// again for every account in a multi-account task.
		for _, row := range taskResultRows(task.Result["tests"]) {
			id, ok := row["account_id"].(string)
			if !ok {
				continue
			}
			stats, ok := result[id]
			if !ok {
				continue
			}
			stats.Add(now, at, modelCheckOutcome(row))
			result[id] = stats
		}
	}
	for id, stats := range result {
		stats.Calculate(true)
		result[id] = stats
	}
	return result
}

func modelCheckOutcome(row map[string]any) string {
	if !modelCheckRequestFailed(row) {
		switch row["verdict"] {
		case "MATCH", "GROUP_MATCH", "SOL_CONSISTENT":
			return "passed"
		case "MISMATCH", "LUNA_LIKE", "TERRA_LIKE":
			return "failed"
		}
	}
	return "inconclusive"
}

func addDetectionConfidence(statistics map[string]accountquality.Statistics, rows []taskstore.DetectionConfidenceResult, now time.Time) {
	for _, row := range rows {
		if !stablePositiveID(row.AccountID) {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, row.CompletedAt)
		if err != nil || at.After(now) || at.Before(now.Add(-30*24*time.Hour)) {
			continue
		}
		stats, exists := statistics[row.AccountID]
		if !exists {
			stats = accountquality.New(now)
		}
		outcome := "inconclusive"
		if row.Status == "succeeded" {
			switch row.Verdict {
			case "passed":
				outcome = "passed"
			case "not_passed":
				outcome = "failed"
			}
		}
		stats.Add(now, at, outcome)
		statistics[row.AccountID] = stats
	}
	for id, stats := range statistics {
		stats.Calculate(true)
		statistics[id] = stats
	}
}
