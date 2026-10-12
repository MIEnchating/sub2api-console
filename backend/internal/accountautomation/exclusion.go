package accountautomation

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func (s *Service) managedAccountIDs(ctx context.Context, ids []string) ([]string, error) {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		err := business.CheckAccountUpstreamManaged(ctx, s.groups, id)
		if errors.Is(err, business.ErrUpstreamExcluded) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func (s *Service) sameManagedAccountIDs(ctx context.Context, saved any, current []string) (bool, error) {
	var ids []string
	switch values := saved.(type) {
	case []string:
		ids = values
	case []any:
		for _, value := range values {
			id, ok := value.(string)
			if !ok {
				return false, nil
			}
			ids = append(ids, id)
		}
	default:
		return false, nil
	}
	ids, err := s.managedAccountIDs(ctx, ids)
	return err == nil && sameAccountIDs(ids, current), err
}
