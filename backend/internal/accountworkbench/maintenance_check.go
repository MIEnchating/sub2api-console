package accountworkbench

import (
	"context"
	"time"
)

func (s *Service) checkMaintenanceCandy(ctx context.Context, account map[string]any, credentials map[string]any, proxyURL string) (map[string]any, error) {
	settings, err := s.ImportModel(ctx)
	if err != nil {
		return nil, err
	}
	return s.checker.CheckOAuthCandyWithProxy(ctx, text(account["id"]), text(account["name"]), credentials, settings.Model, candyCheckTimeoutSeconds, proxyURL)
}

func (s *Service) checkHealthyMaintenanceAccount(ctx context.Context, value *privateMaintenance, account map[string]any, result MaintenanceResult) MaintenanceResult {
	result.Action = "check"
	result.Status, result.Reason = "manual", "check_failed"
	credentials, proxyURL, err := s.GenerationCredentials(ctx, value.Target, account)
	if err != nil {
		return result
	}
	if credentials == nil {
		binding, err := s.accountProxy(ctx, value.Target, account)
		if err != nil {
			return result
		}
		credentials, proxyURL = object(account["credentials"]), proxyBindingURL(binding)
	}
	report, err := s.checkMaintenanceCandy(ctx, account, credentials, proxyURL)
	result.Check = publicCheck(report, credentials)
	if err == nil && passedImportCheck(report) {
		result.Status, result.Reason = "healthy", "check_passed"
	} else {
		next := time.Now().UTC().Add(time.Duration(value.Public.CooldownMinutes) * time.Minute)
		result.NextEligibleAt = &next
	}
	return result
}
