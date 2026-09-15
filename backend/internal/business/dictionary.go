package business

import (
	"context"
	"fmt"
	"sort"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// DictionaryValues reads catalog identities without computing health or scheduling projections.
func (s *Store) DictionaryValues(ctx context.Context, kind string) ([]configstore.DictionaryEntry, error) {
	result := []configstore.DictionaryEntry{}
	switch kind {
	case "group":
		rows, err := s.db.QueryContext(ctx, `SELECT name,remote_id FROM local_groups WHERE remote_id IS NOT NULL AND TRIM(remote_id)<>'' ORDER BY name,remote_id`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var item configstore.DictionaryEntry
			if err := rows.Scan(&item.Name, &item.Value); err != nil {
				return nil, err
			}
			result = append(result, item)
		}
		return result, rows.Err()
	case "platform":
		rows, err := s.db.QueryContext(ctx, `SELECT metadata_json FROM accounts`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		seen := map[string]bool{}
		for rows.Next() {
			var metadata string
			if err := rows.Scan(&metadata); err != nil {
				return nil, err
			}
			platform := accountMetadataText(metadata, "platform")
			if platform == nil {
				continue
			}
			value := *platform
			if seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, configstore.DictionaryEntry{Name: value, Value: value})
		}
		sort.Slice(result, func(i, j int) bool { return result[i].Value < result[j].Value })
		return result, rows.Err()
	default:
		return nil, fmt.Errorf("无效的目录字典类型")
	}
}
