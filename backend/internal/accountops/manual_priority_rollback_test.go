package accountops

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type cancelAfterPriorityReservation struct {
	*business.Store
	cancel context.CancelFunc
}

func (repository *cancelAfterPriorityReservation) AssignManualPriority(ctx context.Context, accountID string, priority int64, loadFactor string, concurrency int64, syncBalanceMultiplier bool, actor string) (business.ManualPriorityAssignment, error) {
	result, err := repository.Store.AssignManualPriority(ctx, accountID, priority, loadFactor, concurrency, syncBalanceMultiplier, actor)
	if err == nil {
		repository.cancel()
	}
	return result, err
}

func TestManualPriorityCancellationBeforeRemoteWriteRestoresLocalReservation(t *testing.T) {
	store, db, _ := accountRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &cancelAfterPriorityReservation{Store: store, cancel: cancel}
	service := New(&testTarget{}, repository, nil)
	config, err := repository.ManualPriorityConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.setManualPriority(ctx, repository, config, "41", 3, "100", 100, true, true, "operator")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reservation error=%v", err)
	}
	var reservations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM manual_priority_accounts WHERE account_id='41'`).Scan(&reservations); err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("cancelled operation left %d reservations without changing remote account", reservations)
	}
}
