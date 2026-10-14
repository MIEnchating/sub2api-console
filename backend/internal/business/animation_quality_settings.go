package business

import "context"

func (s *Store) LoadAnimationQualitySettings(ctx context.Context) ([]byte, error) {
	return s.loadModelCheckState(ctx, "animation-quality-settings")
}
func (s *Store) SaveAnimationQualitySettings(ctx context.Context, raw []byte, actor string) error {
	return s.saveModelCheckState(ctx, raw, actor, "animation.quality.settings.saved", "animation-quality-settings")
}
