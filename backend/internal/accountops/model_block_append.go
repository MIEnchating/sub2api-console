package accountops

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func (s *Service) AddModelSyncBlock(ctx context.Context, model, actor string) (business.AccountModelSyncSettings, error) {
	repository, ok := s.repository.(interface {
		AddAccountModelBlock(context.Context, string, string) (business.AccountModelSyncSettings, error)
	})
	if !ok {
		return business.AccountModelSyncSettings{}, errors.New("账号模型同步存储尚未就绪")
	}
	return repository.AddAccountModelBlock(ctx, model, actor)
}
