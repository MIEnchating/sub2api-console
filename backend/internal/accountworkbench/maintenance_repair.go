package accountworkbench

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func maintenanceConfiguration(account map[string]any) string {
	copy := maps.Clone(account)
	delete(copy, "credentials")
	delete(copy, "status")
	delete(copy, "schedulable")
	return accountVersion(copy)
}
func (s *Service) maintainAccount(ctx context.Context, value *privateMaintenance, listed map[string]any) MaintenanceResult {
	id := text(listed["id"])
	result := MaintenanceResult{AccountID: id, Email: publicAccount(listed).Email, Action: "none", Status: "manual", Reason: "account_changed"}
	if parsed, err := strconv.ParseUint(id, 10, 63); err != nil || parsed == 0 {
		return result
	}
	saved := value.States[id]
	if saved.Status == "blocked" {
		return saved
	}
	if saved.NextEligibleAt != nil && saved.NextEligibleAt.After(time.Now().UTC()) {
		saved.Action = "none"
		return saved
	}
	ctx, release, err := s.acquireGroupMutation(targetguard.Expect(ctx, value.Target), mutationguard.Account(id))
	if err != nil {
		result.Reason = "account_busy"
		return result
	}
	defer release()
	if err = s.requireAccountManaged(ctx, id); err != nil {
		result.Status, result.Reason = "skipped", "upstream_excluded"
		if !errors.Is(err, business.ErrUpstreamExcluded) {
			result.Status, result.Reason = "manual", "verification_failed"
		}
		return result
	}
	if err = s.maintenanceAllowed(ctx, value); err != nil {
		return result
	}
	client, err := s.client(value.Target)
	if err != nil {
		return result
	}
	account, err := client.Account(ctx, id)
	if err != nil {
		result.Reason = "account_read_failed"
		return result
	}
	if accountVersion(account) != accountVersion(listed) || !maintenanceEligible(account, value.Public.GroupIDs) {
		return result
	}
	workspace, user := credentialIdentity(object(account["credentials"]))
	if workspace == "" || user == "" {
		result.Reason = "identity_missing"
		return result
	}
	attempt := value.Attempts[id]
	if attempt != nil && !attempt.ExpiresAt.After(time.Now().UTC()) && workspace == attempt.Workspace && user == attempt.User && digest(object(account["credentials"])) != attempt.CredentialVersion {
		// A subsequently replaced credential is a new source. The old unknown
		// request remains non-replayable when the site's credential is unchanged.
		delete(value.Attempts, id)
		attempt = nil
	}
	if attempt == nil {
		condition := maintenanceCondition(account, time.Now().UTC(), value.Public.IntervalMinutes)
		if condition.Kind == "healthy" && value.Public.CheckEnabled {
			return s.checkHealthyMaintenanceAccount(ctx, value, account, result)
		}
		if condition.Kind != "auth" {
			return maintenanceDecision(value, result, condition)
		}
		binding, proxyErr := s.accountProxy(ctx, value.Target, account)
		if proxyErr != nil {
			result.Reason = "verification_failed"
			return result
		}
		attempt = &maintenanceAttempt{Phase: "refreshing", AccountVersion: accountVersion(account), ConfigurationVersion: maintenanceConfiguration(account), CredentialVersion: digest(object(account["credentials"])), Workspace: workspace, User: user, ExpiresAt: time.Now().UTC().Add(2 * time.Hour)}
		attempt.ProxyBinding = binding
		value.Attempts[id] = attempt
		result.Action = "refresh"
		if err = s.persistMaintenance(value); err != nil {
			result.Reason = "private_save_failed"
			return result
		}
		ctx = adminclient.WithMutationAuthorization(ctx, func(ctx context.Context) error { return s.maintenanceAllowed(ctx, value) })
		_, refreshErr := client.Mutate(ctx, http.MethodPost, "/admin/openai/accounts/"+id+"/refresh", nil)
		account, err = client.Account(ctx, id)
		if err != nil {
			result.Reason = "refresh_unconfirmed"
			return result
		}
		if refreshErr != nil {
			var response *adminclient.HTTPError
			if errors.As(refreshErr, &response) && response.StatusCode == 401 {
				attempt.Phase = "refresh_rejected"
			} else if digest(object(account["credentials"])) == attempt.CredentialVersion {
				result.Reason = "refresh_unconfirmed"
				return result
			}
		}
	}
	if !attempt.ExpiresAt.After(time.Now().UTC()) {
		attempt.Credentials = nil
		result.Reason = "authorization_expired"
		return result
	}
	if attempt.ProxyBinding == nil && text(account["proxy_id"]) != "" {
		result.Reason = "verification_failed"
		return result
	}
	if err = s.verifyPlatformProxy(ctx, value.Target, attempt.ProxyBinding); err != nil {
		result.Reason = "verification_failed"
		return result
	}
	actualWorkspace, actualUser := credentialIdentity(object(account["credentials"]))
	if actualWorkspace != attempt.Workspace || actualUser != attempt.User || maintenanceConfiguration(account) != attempt.ConfigurationVersion {
		result.Reason = "account_changed"
		return result
	}
	if attempt.Phase == "refreshing" {
		if digest(object(account["credentials"])) == attempt.CredentialVersion {
			result.Reason = "refresh_unconfirmed"
			return result
		}
		attempt.Phase = "refreshed"
		attempt.AccountVersion = accountVersion(account)
		if err = s.persistMaintenance(value); err != nil {
			result.Reason = "private_save_failed"
			return result
		}
	}
	if attempt.Phase == "refresh_rejected" {
		result.Action = "reauthorize"
		if err = s.reauthorizeMaintenance(ctx, value, account, attempt); err != nil {
			result.Reason = "manual_login_required"
			return result
		}
	}
	if attempt.Phase == "pending_identity" {
		result.Action = "reauthorize"
		if err = s.verifyMaintenanceAuthorization(ctx, value, account, attempt); err != nil {
			result.Reason = "manual_login_required"
			return result
		}
	}
	if attempt.Phase == "pending_upload" || attempt.Phase == "uploading" {
		result.Action = "reauthorize"
		account, err = s.uploadMaintenanceAuthorization(ctx, value, client, account, attempt)
		if err != nil {
			result.Reason = "upload_unconfirmed"
			return result
		}
	}
	if attempt.Phase == "refreshed" || attempt.Phase == "uploaded" {
		if result.Action == "none" {
			result.Action = "refresh"
		}
		if attempt.Phase == "uploaded" {
			result.Action = "reauthorize"
		}
		if value.Public.CheckAfterRepair {
			proxyURL := proxyBindingURL(attempt.ProxyBinding)
			credentials := object(account["credentials"])
			report, checkErr := s.checkMaintenanceCandy(ctx, account, credentials, proxyURL)
			result.Check = publicCheck(report, credentials)
			if checkErr != nil {
				result.Reason = "verification_failed"
				return result
			}
			if !passedImportCheck(report) {
				if attempt.Phase == "refreshed" && ClassifyMaintenanceSignal(report).Kind == "auth" {
					result.Action = "reauthorize"
					if err = s.reauthorizeMaintenance(ctx, value, account, attempt); err != nil {
						result.Reason = "manual_login_required"
						return result
					}
					account, err = s.uploadMaintenanceAuthorization(ctx, value, client, account, attempt)
					if err != nil {
						result.Reason = "upload_unconfirmed"
						return result
					}
					credentials = object(account["credentials"])
					report, checkErr = s.checkMaintenanceCandy(ctx, account, credentials, proxyURL)
					result.Check = publicCheck(report, credentials)
				}
				if checkErr != nil || !passedImportCheck(report) {
					result.Reason = "verification_failed"
					return result
				}
			}
		}
		attempt.Phase = "verified"
		attempt.AccountVersion = accountVersion(account)
		attempt.VerifiedCredentialsVersion = digest(object(account["credentials"]))
		if err = s.persistMaintenance(value); err != nil {
			result.Reason = "private_save_failed"
			return result
		}
	}
	if attempt.Phase == "recovering" {
		if account["schedulable"] == true && text(account["status"]) == "active" && digest(object(account["credentials"])) == attempt.VerifiedCredentialsVersion {
			delete(value.Attempts, id)
			result.Status = "repaired"
			result.Reason = "authorization_repaired"
			return result
		}
		result.Reason = "recovery_unconfirmed"
		return result
	}
	if attempt.Phase != "verified" {
		result.Reason = "manual_login_required"
		return result
	}
	current, err := client.Account(ctx, id)
	if err != nil || accountVersion(current) != attempt.AccountVersion {
		result.Reason = "account_changed"
		return result
	}
	attempt.Phase = "recovering"
	if err = s.verifyPlatformProxy(ctx, value.Target, attempt.ProxyBinding); err != nil {
		result.Reason = "verification_failed"
		return result
	}
	if err = s.persistMaintenance(value); err != nil {
		result.Reason = "private_save_failed"
		return result
	}
	ctx = adminclient.WithMutationAuthorization(ctx, func(ctx context.Context) error { return s.maintenanceAllowed(ctx, value) })
	if _, err = client.Mutate(ctx, http.MethodPost, "/admin/accounts/"+id+"/clear-error", map[string]any{}); err != nil {
		result.Reason = "recovery_unconfirmed"
		return result
	}
	if _, err = client.SetAccountSchedulable(ctx, id, true); err != nil {
		result.Reason = "recovery_unconfirmed"
		return result
	}
	current, err = client.Account(ctx, id)
	if err != nil || current["schedulable"] != true || text(current["status"]) != "active" || !sameAccountIdentity(current, object(account["credentials"])) || maintenanceConfiguration(current) != attempt.ConfigurationVersion {
		result.Reason = "recovery_unconfirmed"
		return result
	}
	delete(value.Attempts, id)
	result.Status = "repaired"
	result.Reason = "authorization_repaired"
	return result
}
func maintenanceDecision(value *privateMaintenance, result MaintenanceResult, condition MaintenanceCondition) MaintenanceResult {
	result.Reason = condition.Reason
	switch condition.Kind {
	case "healthy":
		result.Status = "healthy"
	case "permanent":
		result.Status = "blocked"
	case "transient":
		result.Status = "cooldown"
		next := time.Now().UTC().Add(time.Duration(value.Public.CooldownMinutes) * time.Minute)
		result.NextEligibleAt = &next
	default:
		result.Status = "manual"
	}
	return result
}
