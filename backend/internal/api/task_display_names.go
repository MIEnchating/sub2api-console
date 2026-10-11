package api

import (
	"context"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type taskDetailResponse struct {
	taskstore.Task
	// Current directory labels are presentation metadata, never historical results.
	DisplayAccountNames map[string]string `json:"display_account_names,omitempty"`
}

type taskAccountLabelReader interface {
	AccountLogLabels(context.Context, []string) (map[string]*business.AccountLogLabel, error)
}

func (s *Server) taskDisplayAccountNames(ctx context.Context, task taskstore.Task) map[string]string {
	if task.Operation != "active-probe" {
		return nil
	}
	reader, ok := s.business.(taskAccountLabelReader)
	if !ok {
		return nil
	}
	rows, _ := task.Result["results"].([]any)
	seen := make(map[string]bool)
	ids := make([]string, 0)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id, _ := row["account_id"].(string)
		name, _ := row["account_name"].(string)
		if name != "" || id == "" || len(id) > 255 || id != strings.TrimSpace(id) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	names := make(map[string]string)
	// Bound SQLite parameters even for large legacy results; never load all accounts.
	const batchSize = 200
	for start := 0; start < len(ids); start += batchSize {
		batch := ids[start:min(start+batchSize, len(ids))]
		labels, err := reader.AccountLogLabels(ctx, batch)
		if err != nil {
			return names
		}
		for _, id := range batch {
			if label := labels[id]; label != nil && label.Name != "" {
				names[id] = label.Name
			}
		}
	}
	return names
}
