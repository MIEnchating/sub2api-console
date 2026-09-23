package api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type detectionScopeReader interface {
	DetectionTaskScope(context.Context, []string) (business.DetectionTaskScope, error)
}

// Legacy tasks have no membership snapshot. Only enrich the response and clearly
// identify the current directory as the source; never rewrite their history.
func (s *Server) detectionTaskDisplayGroups(ctx context.Context, task taskstore.Task) taskstore.Task {
	if task.Operation != "managed-model-detection" {
		return task
	}
	if _, exists := task.Result["group_ids_by_account"]; exists {
		return task
	}
	raw, err := json.Marshal(task.Result["configuration"])
	if err != nil {
		return task
	}
	var configuration struct {
		GroupIDs []string `json:"group_ids"`
	}
	if json.Unmarshal(raw, &configuration) != nil || len(configuration.GroupIDs) == 0 || len(configuration.GroupIDs) > 100 {
		return task
	}
	for _, id := range configuration.GroupIDs {
		value, err := strconv.ParseUint(id, 10, 64)
		if err != nil || value == 0 || strconv.FormatUint(value, 10) != id {
			return task
		}
	}
	reader, ok := s.business.(detectionScopeReader)
	if !ok {
		return task
	}
	raw, err = json.Marshal(task.Result["account_ids"])
	if err != nil {
		return task
	}
	var accountIDs []string
	if json.Unmarshal(raw, &accountIDs) != nil || len(accountIDs) == 0 {
		return task
	}
	result := make(map[string]any, len(task.Result)+3)
	for key, value := range task.Result {
		result[key] = value
	}
	task.Result = result
	scope, err := reader.DetectionTaskScope(ctx, configuration.GroupIDs)
	if err != nil {
		task.Result["grouping_source"] = "unavailable"
		return task
	}
	memberships := make(map[string][]string, len(accountIDs))
	for _, id := range accountIDs {
		memberships[id] = scope.GroupIDsByAccount[id]
	}
	task.Result["group_ids_by_account"] = memberships
	task.Result["group_names_by_id"] = scope.GroupNamesByID
	task.Result["grouping_source"] = "current"
	return task
}
