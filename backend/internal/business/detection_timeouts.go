package business

import "context"

func (s *Store) LoadDetectionTimeouts(ctx context.Context) ([]byte, error) {
	return s.loadModelCheckState(ctx, "model-detection-timeouts")
}
func (s *Store) SaveDetectionTimeouts(ctx context.Context, raw []byte, actor string) error {
	return s.saveModelCheckState(ctx, raw, actor, "detection.timeouts.saved", "model-detection-timeouts")
}
