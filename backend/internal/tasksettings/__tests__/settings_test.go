package tasksettings_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/tasksettings"
)

func TestSettingsPersistReloadAndRejectStaleOrInvalidChanges(t *testing.T) {
	ctx := context.Background()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	g := taskrunner.NewQueued(ctx, 2, 100)
	defer g.Cancel()
	service, err := tasksettings.New(ctx, store, map[string]*taskrunner.Group{"account": g})
	if err != nil {
		t.Fatal(err)
	}
	initial := service.Snapshot()
	updated, err := service.Update(ctx, tasksettings.Input{Limits: map[string]int{"account": 3}, QueueCapacity: 25, Version: initial.Version})
	if err != nil {
		t.Fatal(err)
	}
	if g.Snapshot().Limit != 3 || g.Snapshot().QueueCapacity != 25 {
		t.Fatal(g.Snapshot())
	}
	_, err = service.Update(ctx, tasksettings.Input{Limits: map[string]int{"account": 4}, QueueCapacity: 25, Version: initial.Version})
	if !errors.Is(err, tasksettings.ErrConflict) {
		t.Fatal(err)
	}
	for _, limits := range []map[string]int{{"account": 0}, {"account": 10001}, {"missing": 1}, {}} {
		_, err = service.Update(ctx, tasksettings.Input{Limits: limits, QueueCapacity: 25, Version: updated.Version})
		if !errors.Is(err, tasksettings.ErrInvalid) {
			t.Fatalf("invalid input %v: %v", limits, err)
		}
	}
	restarted := taskrunner.NewQueued(ctx, 8, 100)
	defer restarted.Cancel()
	loaded, err := tasksettings.New(ctx, store, map[string]*taskrunner.Group{"account": restarted})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Snapshot().Version != updated.Version || restarted.Snapshot().Limit != 3 || restarted.Snapshot().QueueCapacity != 25 {
		t.Fatal(loaded.Snapshot())
	}
}

type failedStore struct{}

func (failedStore) TaskLimits(context.Context) (string, error) { return "", nil }
func (failedStore) SaveTaskLimits(context.Context, string, string) error {
	return errors.New("disk unavailable")
}
func TestFailedPersistenceDoesNotChangeActiveLimits(t *testing.T) {
	g := taskrunner.NewQueued(context.Background(), 2, 100)
	defer g.Cancel()
	s, err := tasksettings.New(context.Background(), failedStore{}, map[string]*taskrunner.Group{"account": g})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(context.Background(), tasksettings.Input{Limits: map[string]int{"account": 9}, QueueCapacity: 20, Version: s.Snapshot().Version})
	if err == nil || g.Snapshot().Limit != 2 || g.Snapshot().QueueCapacity != 100 {
		t.Fatalf("failed save changed limits: %v %v", err, g.Snapshot())
	}
}
