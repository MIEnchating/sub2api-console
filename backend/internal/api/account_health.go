package api

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type accountReadSnapshotter interface {
	WithReadSnapshot(context.Context, func(context.Context) error) error
}

func (s *Server) withAccountReadSnapshot(ctx context.Context, read func(context.Context) error) error {
	if snapshotter, ok := s.business.(accountReadSnapshotter); ok {
		return snapshotter.WithReadSnapshot(ctx, read)
	}
	return read(ctx)
}

func (s *Server) readAccountResultsSnapshot(ctx context.Context, reader accountResultsReader, id string) (accountResultsSnapshot, error) {
	snapshot := accountResultsSnapshot{AccountID: id}
	err := s.withAccountReadSnapshot(ctx, func(ctx context.Context) error {
		results, err := reader.RecentAccountResults(ctx, id, 100)
		if err != nil {
			return err
		}
		s.enrichRecentResults(ctx, []business.AccountStatus{{RecentResults: results}})
		snapshot.Results = results
		if s.accountHealth == nil {
			return nil
		}
		health, err := s.accountHealth.ProjectHealth(ctx, routing.Scope{AccountID: &id})
		if err != nil {
			return err
		}
		if projection, found := health[id]; found {
			snapshot.Health = &projection
		}
		return nil
	})
	return snapshot, err
}

func (s *Server) enrichAccountHealth(ctx context.Context, accounts []business.AccountStatus) error {
	if s.accountHealth == nil || len(accounts) == 0 {
		return nil
	}
	scope := routing.Scope{}
	if len(accounts) == 1 {
		scope.AccountID = &accounts[0].ID
	}
	health, err := s.accountHealth.ProjectHealth(ctx, scope)
	if err != nil {
		return err
	}
	for index := range accounts {
		projection, found := health[accounts[index].ID]
		if !found {
			continue
		}
		account := &accounts[index]
		account.HealthScore, account.ShortScore, account.LongScore = projection.HealthScore, projection.ShortScore, projection.LongScore
		account.SampleCount, account.ShortSampleCount, account.LongSampleCount = projection.SampleCount, projection.ShortSampleCount, projection.LongSampleCount
		account.TTFBP50MS, account.TTFBP95MS = projection.TTFBP50MS, projection.TTFBP95MS
		account.HealthEvaluatedAt, account.HealthEvidenceAt = &projection.HealthEvaluatedAt, projection.HealthEvidenceAt
	}
	return nil
}
