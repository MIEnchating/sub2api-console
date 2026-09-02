package business

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const mutationLeasePrefix = "mutation/"

const mutationLeaseSQLChunk = 500

func (s *Store) ResolveMutationResources(ctx context.Context, resources []string) ([]string, error) {
	resolved := make([]string, 0, len(resources)*2)
	hosts := []string{}
	catalogRequested := false
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if !strings.HasPrefix(resource, "upstream/") {
			resolved = append(resolved, resource)
			catalogRequested = catalogRequested || resource == "upstream-catalog"
			continue
		}
		host := canonicalHost(strings.TrimPrefix(resource, "upstream/"))
		if host == "" {
			return nil, errors.New("变更租约上游 Host 无效")
		}
		hosts = append(hosts, host)
	}
	if len(hosts) == 0 && !catalogRequested {
		return resolved, nil
	}
	state, err := loadUpstreamIdentityState(ctx, s.db)
	if err != nil {
		return nil, err
	}
	components, componentsByHost := state.componentIndex()
	componentsToReconcile := map[string][]string{}
	stableHosts := map[string]string{}
	requiresCatalog := catalogRequested
	for _, host := range hosts {
		component := componentsByHost[host]
		if len(component) == 0 {
			component = []string{host}
		}
		componentKey := strings.Join(component, "\x00")
		if state.componentNeedsReconciliation(component) {
			requiresCatalog = true
			componentsToReconcile[componentKey] = component
			continue
		}
		mapping, found := state.existing[host]
		if !found {
			return nil, errors.New("稳定上游身份解析结果不完整")
		}
		upstreamID := strings.TrimSpace(mapping.id)
		if upstreamID == "" {
			return nil, errors.New("稳定上游身份缺少 ID")
		}
		stableHosts[host] = upstreamID
	}
	if requiresCatalog {
		resolved = append(resolved, "upstream-catalog")
		for _, component := range components {
			if state.componentNeedsReconciliation(component) {
				componentsToReconcile[strings.Join(component, "\x00")] = component
			}
		}
	}
	accountHosts := map[string]struct{}{}
	accountIdentityIDs := map[string]struct{}{}
	for host, upstreamID := range stableHosts {
		resolved = append(resolved, "upstream/"+host)
		resolved = append(resolved, "upstream-identity/"+upstreamID)
		if requiresCatalog {
			accountIdentityIDs[upstreamID] = struct{}{}
			for _, identityHost := range state.identityHosts[upstreamID] {
				resolved = append(resolved, "upstream/"+identityHost)
				accountHosts[identityHost] = struct{}{}
			}
		}
	}
	for _, componentKey := range sortedStringMapKeys(componentsToReconcile) {
		component := componentsToReconcile[componentKey]
		for _, candidate := range component {
			resolved = append(resolved, "upstream/"+candidate)
			accountHosts[candidate] = struct{}{}
			mapping, found := state.existing[candidate]
			if !found {
				continue
			}
			upstreamID := strings.TrimSpace(mapping.id)
			if upstreamID == "" {
				return nil, errors.New("稳定上游身份缺少 ID")
			}
			resolved = append(resolved, "upstream-identity/"+upstreamID)
			accountIdentityIDs[upstreamID] = struct{}{}
			for _, identityHost := range state.identityHosts[upstreamID] {
				resolved = append(resolved, "upstream/"+identityHost)
				accountHosts[identityHost] = struct{}{}
			}
		}
	}
	if len(accountHosts) > 0 || len(accountIdentityIDs) > 0 {
		accountIDs, err := s.upstreamIdentityMutationAccountIDs(
			ctx, sortedStringMapKeys(accountHosts), sortedStringMapKeys(accountIdentityIDs),
		)
		if err != nil {
			return nil, err
		}
		for _, accountID := range accountIDs {
			resolved = append(resolved, "account/"+accountID)
		}
	}
	return resolved, nil
}

func (s *Store) upstreamIdentityMutationAccountIDs(ctx context.Context, hosts, upstreamIDs []string) ([]string, error) {
	hostValues, err := json.Marshal(hosts)
	if err != nil {
		return nil, err
	}
	identityValues, err := json.Marshal(upstreamIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH
		host_values(value) AS (SELECT CAST(value AS TEXT) FROM json_each(?)),
		identity_values(value) AS (SELECT CAST(value AS TEXT) FROM json_each(?))
		SELECT DISTINCT account_id FROM (
			SELECT id AS account_id FROM accounts WHERE upstream_host IN (SELECT value FROM host_values)
			UNION ALL
			SELECT local_account_id AS account_id FROM bindings WHERE upstream_host IN (SELECT value FROM host_values)
			UNION ALL
			SELECT b.local_account_id AS account_id FROM binding_identities bi
			JOIN bindings b ON b.id=bi.binding_id WHERE bi.upstream_id IN (SELECT value FROM identity_values)
		) WHERE TRIM(account_id)<>'' ORDER BY account_id`, string(hostValues), string(identityValues))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			return nil, errors.New("稳定上游关联账号缺少 ID")
		}
		result = append(result, accountID)
	}
	return result, rows.Err()
}

// AcquireMutationLease atomically reserves every resource or reserves none.
func (s *Store) AcquireMutationLease(
	ctx context.Context,
	ownerID string,
	resources []string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	ownerID = strings.TrimSpace(ownerID)
	resources, err := mutationLeaseNames(resources)
	if err != nil || ownerID == "" || ttl < time.Second {
		if err != nil {
			return false, err
		}
		return false, errors.New("变更租约参数无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, preferContextError(ctx, err)
	}
	defer tx.Rollback()

	current := now.UTC()
	for start := 0; start < len(resources); start += mutationLeaseSQLChunk {
		chunk := resources[start:min(start+mutationLeaseSQLChunk, len(resources))]
		placeholders, arguments := mutationLeaseArguments(chunk)
		rows, err := tx.QueryContext(ctx, `SELECT lease_name,owner_id,expires_at FROM scheduler_leases
			WHERE lease_name IN (`+placeholders+`)`, arguments...)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var leaseName, currentOwner, expiresText string
			if err := rows.Scan(&leaseName, &currentOwner, &expiresText); err != nil {
				rows.Close()
				return false, err
			}
			expiresAt, parseErr := time.Parse(time.RFC3339Nano, expiresText)
			if parseErr != nil {
				rows.Close()
				return false, fmt.Errorf("变更租约 %s 的到期时间无效：%w", leaseName, parseErr)
			}
			if currentOwner != ownerID && expiresAt.After(current) {
				rows.Close()
				return false, nil
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, err
		}
		if err := rows.Close(); err != nil {
			return false, err
		}
	}
	for start := 0; start < len(resources); start += mutationLeaseSQLChunk {
		chunk := resources[start:min(start+mutationLeaseSQLChunk, len(resources))]
		placeholders, arguments := mutationLeaseArguments(chunk)
		if _, err := tx.ExecContext(ctx, `DELETE FROM scheduler_leases WHERE lease_name IN (`+placeholders+`)`, arguments...); err != nil {
			return false, err
		}
	}
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}
	formattedNow := current.Format(time.RFC3339Nano)
	formattedExpiry := current.Add(ttl).Format(time.RFC3339Nano)
	for _, resource := range resources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO scheduler_leases(
			lease_name,owner_id,owner_pid,owner_host,checked_at,acquired_at,renewed_at,expires_at
		) VALUES(?,?,?,?,?,?,?,?)`, resource, ownerID, os.Getpid(), hostname,
			formattedNow, formattedNow, formattedNow, formattedExpiry); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) RenewMutationLease(
	ctx context.Context,
	ownerID string,
	resources []string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	ownerID = strings.TrimSpace(ownerID)
	resources, err := mutationLeaseNames(resources)
	if err != nil || ownerID == "" || ttl < time.Second {
		if err != nil {
			return false, err
		}
		return false, errors.New("变更租约参数无效")
	}
	current := now.UTC()
	formattedNow := current.Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var count int64
	for start := 0; start < len(resources); start += mutationLeaseSQLChunk {
		chunk := resources[start:min(start+mutationLeaseSQLChunk, len(resources))]
		placeholders, resourceArguments := mutationLeaseArguments(chunk)
		arguments := []any{formattedNow, current.Add(ttl).Format(time.RFC3339Nano), ownerID, formattedNow}
		arguments = append(arguments, resourceArguments...)
		result, err := tx.ExecContext(ctx, `UPDATE scheduler_leases SET renewed_at=?,expires_at=?
			WHERE owner_id=? AND julianday(expires_at)>julianday(?) AND lease_name IN (`+placeholders+`)`, arguments...)
		if err != nil {
			return false, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		count += changed
	}
	if count != int64(len(resources)) {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ReleaseMutationLease(ctx context.Context, ownerID string, resources []string) error {
	ownerID = strings.TrimSpace(ownerID)
	resources, err := mutationLeaseNames(resources)
	if err != nil {
		return err
	}
	if ownerID == "" {
		return errors.New("变更租约参数无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for start := 0; start < len(resources); start += mutationLeaseSQLChunk {
		chunk := resources[start:min(start+mutationLeaseSQLChunk, len(resources))]
		placeholders, resourceArguments := mutationLeaseArguments(chunk)
		arguments := []any{ownerID}
		arguments = append(arguments, resourceArguments...)
		if _, err := tx.ExecContext(ctx, `DELETE FROM scheduler_leases WHERE owner_id=? AND lease_name IN (`+placeholders+`)`, arguments...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func mutationLeaseArguments(resources []string) (string, []any) {
	arguments := make([]any, len(resources))
	for index := range resources {
		arguments[index] = resources[index]
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(resources)), ","), arguments
}

func mutationLeaseNames(resources []string) ([]string, error) {
	unique := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		resource = strings.TrimSpace(resource)
		if resource == "" || len(resource) > 512 {
			return nil, errors.New("变更租约资源无效")
		}
		unique[mutationLeasePrefix+resource] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, errors.New("变更租约至少需要一个资源")
	}
	result := make([]string, 0, len(unique))
	for resource := range unique {
		result = append(result, resource)
	}
	sort.Strings(result)
	return result, nil
}

var _ interface {
	AcquireMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error)
	RenewMutationLease(context.Context, string, []string, time.Time, time.Duration) (bool, error)
	ReleaseMutationLease(context.Context, string, []string) error
} = (*Store)(nil)
