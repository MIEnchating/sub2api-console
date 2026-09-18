package modelcheck

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/accountquality"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"time"
)

func accountConfidence(tasks []taskstore.Task, now time.Time) map[string]accountquality.Statistics {
	result := map[string]accountquality.Statistics{}
	for _, task := range tasks {
		if task.Status == "queued" || task.Status == "running" || task.Status == "waiting_input" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, task.UpdatedAt)
		for _, id := range taskAccountIDs(task) {
			stats, ok := result[id]
			if !ok {
				stats = accountquality.New(now)
			}
			if err == nil {
				for _, row := range taskResultRows(task.Result["tests"]) {
					if row["account_id"] != id {
						continue
					}
					outcome := "inconclusive"
					if !modelCheckRequestFailed(row) {
						switch row["verdict"] {
						case "MATCH", "GROUP_MATCH", "SOL_CONSISTENT":
							outcome = "passed"
						case "MISMATCH", "LUNA_LIKE", "TERRA_LIKE":
							outcome = "failed"
						}
					}
					stats.Add(now, at, outcome)
				}
			}
			result[id] = stats
		}
	}
	for id, stats := range result {
		stats.Calculate(true)
		result[id] = stats
	}
	return result
}
