package business

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type GroupProbeModels struct {
	GroupID            string   `json:"group_id"`
	GroupName          string   `json:"group_name"`
	Models             []string `json:"models"`
	AccountCount       int      `json:"account_count"`
	AccountsWithModels int      `json:"accounts_with_models"`
	Complete           bool     `json:"complete"`
}

func (s *Store) GroupProbeModels(ctx context.Context, groupID string) (GroupProbeModels, error) {
	if !positiveNumericID(groupID) {
		return GroupProbeModels{}, fmt.Errorf("分组必须使用已登记的稳定数字 ID")
	}
	group, err := s.groupByID(ctx, groupID)
	if err != nil {
		return GroupProbeModels{}, err
	}
	result := GroupProbeModels{
		GroupID: groupID, GroupName: group.Name, Models: []string{},
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.metadata_json
		FROM account_groups ag JOIN accounts a ON a.id=ag.account_id
		WHERE ag.group_id=? OR (ag.group_id IS NULL AND LOWER(TRIM(ag.group_name))=LOWER(TRIM(?)))
		ORDER BY CAST(a.id AS INTEGER),a.id`, groupID, group.Name)
	if err != nil {
		return GroupProbeModels{}, err
	}
	defer rows.Close()
	for rows.Next() {
		result.AccountCount++
		var rawMetadata string
		if err := rows.Scan(&rawMetadata); err != nil {
			return GroupProbeModels{}, err
		}
		metadata, err := decodeJSONObject(rawMetadata)
		if err != nil {
			continue
		}
		models := metadataStringList(metadata["known_models"])
		if len(models) == 0 {
			continue
		}
		result.AccountsWithModels++
		if result.AccountsWithModels == 1 {
			result.Models = append(result.Models, models...)
			continue
		}
		result.Models = intersectProbeModels(result.Models, models)
	}
	if err := rows.Err(); err != nil {
		return GroupProbeModels{}, err
	}
	sort.SliceStable(result.Models, func(left, right int) bool {
		return strings.ToLower(result.Models[left]) < strings.ToLower(result.Models[right])
	})
	result.Complete = result.AccountCount > 0 && result.AccountsWithModels == result.AccountCount
	return result, nil
}

func intersectProbeModels(current, candidate []string) []string {
	available := make(map[string]struct{}, len(candidate))
	for _, model := range candidate {
		available[strings.ToLower(strings.TrimSpace(model))] = struct{}{}
	}
	result := make([]string, 0, len(current))
	for _, model := range current {
		if _, present := available[strings.ToLower(strings.TrimSpace(model))]; present {
			result = append(result, model)
		}
	}
	return result
}
