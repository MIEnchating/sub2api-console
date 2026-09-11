package newapimanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

// Every intermediate state must supply an expression for each enabled tiered
// model. Retain old expressions until their modes have been disabled, including
// when a batch enables some models and disables others.
func (s *Service) writeBillingOptions(ctx context.Context, platform configstore.NewAPIPlatform, previous map[string]string, modes, expressions map[string]string) error {
	const modeKey = "billing_setting.billing_mode"
	const exprKey = "billing_setting.billing_expr"
	prepared, err := decodeStringMap(previous[exprKey])
	if err != nil {
		return err
	}
	for model, expression := range expressions {
		prepared[model] = expression
	}
	type write struct{ key, value, before string }
	planned := []struct {
		key    string
		values map[string]string
	}{{exprKey, prepared}, {modeKey, modes}, {exprKey, expressions}}
	current := map[string]string{exprKey: previous[exprKey], modeKey: previous[modeKey]}
	for key, value := range current {
		if value == "" {
			current[key] = "{}"
		}
	}
	writes := make([]write, 0, len(planned))
	for _, step := range planned {
		raw, err := json.Marshal(step.values)
		if err != nil {
			return err
		}
		value := string(raw)
		writes = append(writes, write{step.key, value, current[step.key]})
		current[step.key] = value
	}
	for i, step := range writes {
		_, err := s.request(ctx, platform, "PUT", "/api/option/", map[string]any{"key": step.key, "value": step.value})
		if err == nil {
			continue
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		// Include the failed write: an error response can follow a committed update.
		// Restore each step's immediate predecessor, not the original expression map,
		// which can be invalid while newly enabled modes are still active.
		for j := i; j >= 0; j-- {
			restore := writes[j]
			if _, rollbackErr := s.request(rollbackCtx, platform, "PUT", "/api/option/", map[string]any{"key": restore.key, "value": restore.before}); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("%s 回滚失败：%w", restore.key, rollbackErr))
			}
		}
		cancel()
		return err
	}
	return nil
}
