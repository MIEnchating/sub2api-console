package configstore

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ModelPricingCache struct {
	Content   string
	FetchedAt time.Time
}

func (s *Store) ModelPricingCache(ctx context.Context, key string) (*ModelPricingCache, error) {
	var item ModelPricingCache
	var timestamp string
	err := s.db.QueryRowContext(ctx, `SELECT content,fetched_at FROM model_pricing_cache WHERE cache_key=?`, key).Scan(&item.Content, &timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.FetchedAt, err = time.Parse(time.RFC3339Nano, timestamp)
	return &item, err
}

func (s *Store) SaveModelPricingCache(ctx context.Context, key string, item ModelPricingCache) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO model_pricing_cache(cache_key,content,fetched_at) VALUES(?,?,?)
		ON CONFLICT(cache_key) DO UPDATE SET content=excluded.content,fetched_at=excluded.fetched_at`, key, item.Content, item.FetchedAt.UTC().Format(time.RFC3339Nano))
	return err
}
