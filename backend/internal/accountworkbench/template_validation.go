package accountworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Retain only one successful content fingerprint, never the library or its
// configuration. Every authorization check still reads the current database.
type templateValidation struct {
	mu     sync.Mutex
	digest [sha256.Size]byte
	valid  bool
}

func (s *Service) templateRevision(ctx context.Context, target configstore.TargetSettings) (int64, error) {
	raw, revision, err := s.private.WorkbenchDocument(ctx, "templates:"+targetKey(target))
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return revision, nil
	}
	fingerprint := sha256.Sum256(raw)
	s.templateValidation.mu.Lock()
	defer s.templateValidation.mu.Unlock()
	if !s.templateValidation.valid || s.templateValidation.digest != fingerprint {
		var library TemplateLibrary
		if err := json.Unmarshal(raw, &library); err != nil {
			return 0, errors.New("模板记录无法读取")
		}
		s.templateValidation.digest = fingerprint
		s.templateValidation.valid = true
	}
	return revision, nil
}
