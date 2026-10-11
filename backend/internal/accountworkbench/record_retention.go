package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) pruneRecords(ctx context.Context, owner string) error {
	type candidate struct {
		key     string
		created time.Time
	}
	candidates := []candidate{}
	err := s.documents(ctx, "run:"+owner+":", func(record configstore.WorkbenchDocumentRecord) error {
		// Retention depends only on metadata. Decode private execution data only
		// when re-reading a selected record for associated-resource deletion.
		var value struct {
			Public struct {
				ID        string    `json:"id"`
				Action    string    `json:"action"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"public"`
		}
		if json.Unmarshal(record.Payload, &value) != nil {
			return errors.New("处理记录无法读取，请重新输入账号资料")
		}
		s.activeMu.Lock()
		active := s.active[value.Public.ID] != nil
		s.activeMu.Unlock()
		if !active && value.Public.Action != "maintenance" {
			candidates = append(candidates, candidate{record.ID, value.Public.CreatedAt})
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.SortFunc(candidates, func(a, b candidate) int { return b.created.Compare(a.created) })
	for i := 100; i < len(candidates); i++ {
		raw, revision, err := s.private.WorkbenchDocument(ctx, candidates[i].key)
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}
		value, err := decodeRun(raw)
		if err != nil {
			return err
		}
		value.Public.Revision = revision
		if err = s.deleteRecord(ctx, &value); err != nil {
			return err
		}
	}
	return nil
}
