package accountops

import (
	"errors"
	"fmt"
)

// Validate the parameters both before enabling scheduling and after syncing it.
// Only a valid but mismatching schedulable value may use the Console fallback.
func verifyAccountSettingsParameters(account map[string]any, input SettingsInput) error {
	patch := FieldPatch{
		PriorityPresent: true, Priority: &input.Priority,
		LoadFactorPresent: true, LoadFactor: &input.LoadFactor,
		ConcurrencyPresent: true, Concurrency: &input.Concurrency,
	}
	expectedLoadFactor := &input.LoadFactor
	if input.FollowConcurrency {
		expectedLoadFactor = nil
	}
	if err := verifyFieldReadback(account, nil, &input.Priority, expectedLoadFactor, &input.Concurrency, nil, nil, nil, patch); err != nil {
		return err
	}
	if input.FollowConcurrency {
		loadFactor, err := accountSettingsLoadFactor(account)
		if err != nil || loadFactor != "0" {
			return errors.New("账号负载因子未恢复为跟随并发上限")
		}
	}
	return nil
}

func verifyAccountSettingsRollback(account, expected map[string]any) error {
	// Normalize nullable load factors exactly as when capturing the rollback.
	actual, err := accountSettingsRollbackBody(account)
	if err != nil {
		return err
	}
	for _, field := range []struct{ key, label string }{
		{"priority", "优先级"}, {"load_factor", "负载因子"},
		{"concurrency", "并发上限"}, {"schedulable", "可调度状态"},
	} {
		if actual[field.key] != expected[field.key] {
			return fmt.Errorf("账号%s未恢复原值", field.label)
		}
	}
	return nil
}
