package business

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

type upstreamIdentityMapping struct {
	id        string
	createdAt string
	primary   bool
}

type upstreamIdentityQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type upstreamIdentityState struct {
	graph            map[string]map[string]struct{}
	storedHosts      map[string]struct{}
	preferredPrimary map[string]struct{}
	existing         map[string]upstreamIdentityMapping
	identityHosts    map[string][]string
}

func (s *Store) ensureUpstreamIdentities(ctx context.Context) error {
	state, err := loadUpstreamIdentityState(ctx, s.db)
	if err != nil {
		return err
	}
	hosts := state.reconciliationHosts()
	if len(hosts) == 0 {
		return nil
	}
	resources := make([]string, 0, len(hosts)+1)
	resources = append(resources, mutationguard.UpstreamCatalog())
	for _, host := range hosts {
		resources = append(resources, mutationguard.Upstream(host))
	}
	guarded, release, err := mutationguard.Acquire(ctx, s, resources...)
	if err != nil {
		return err
	}
	reconcileErr := s.reconcileUpstreamIdentities(guarded)
	return errors.Join(reconcileErr, release())
}

func (s *Store) reconcileUpstreamIdentities(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, err := loadUpstreamIdentityState(ctx, tx)
	if err != nil {
		return err
	}
	for _, component := range state.reconciliationComponents() {
		if err := reconcileUpstreamIdentityComponent(
			ctx, tx, component, state.storedHosts, state.preferredPrimary, state.existing,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadUpstreamIdentityState(ctx context.Context, queryer upstreamIdentityQueryer) (upstreamIdentityState, error) {
	state := upstreamIdentityState{
		graph: map[string]map[string]struct{}{}, storedHosts: map[string]struct{}{},
		preferredPrimary: map[string]struct{}{}, existing: map[string]upstreamIdentityMapping{},
		identityHosts: map[string][]string{},
	}
	rows, err := queryer.QueryContext(ctx, `SELECT host,metadata_json FROM upstreams ORDER BY host`)
	if err != nil {
		return upstreamIdentityState{}, err
	}
	for rows.Next() {
		var host, metadataRaw string
		if err := rows.Scan(&host, &metadataRaw); err != nil {
			rows.Close()
			return upstreamIdentityState{}, err
		}
		host = canonicalHost(host)
		if host == "" {
			continue
		}
		state.storedHosts[host] = struct{}{}
		if state.graph[host] == nil {
			state.graph[host] = map[string]struct{}{}
		}
		metadata, decodeErr := decodeObject(metadataRaw)
		if decodeErr != nil {
			continue
		}
		aliases, ok := metadata["alias_hosts"].([]any)
		if !ok || len(aliases) == 0 {
			continue
		}
		state.preferredPrimary[host] = struct{}{}
		for _, rawAlias := range aliases {
			alias, ok := rawAlias.(string)
			if !ok {
				continue
			}
			alias = canonicalHost(alias)
			if alias == "" || alias == host {
				continue
			}
			if state.graph[alias] == nil {
				state.graph[alias] = map[string]struct{}{}
			}
			state.graph[host][alias] = struct{}{}
			state.graph[alias][host] = struct{}{}
		}
	}
	if err := rows.Close(); err != nil {
		return upstreamIdentityState{}, err
	}
	if err := rows.Err(); err != nil {
		return upstreamIdentityState{}, err
	}

	rows, err = queryer.QueryContext(ctx, `SELECT h.host,h.upstream_id,h.is_primary,i.created_at
		FROM upstream_identity_hosts h JOIN upstream_identities i ON i.upstream_id=h.upstream_id`)
	if err != nil {
		return upstreamIdentityState{}, err
	}
	for rows.Next() {
		var host string
		var primary int64
		var mapping upstreamIdentityMapping
		if err := rows.Scan(&host, &mapping.id, &primary, &mapping.createdAt); err != nil {
			rows.Close()
			return upstreamIdentityState{}, err
		}
		mapping.primary = primary == 1
		host = canonicalHost(host)
		state.existing[host] = mapping
		state.identityHosts[mapping.id] = append(state.identityHosts[mapping.id], host)
	}
	if err := rows.Close(); err != nil {
		return upstreamIdentityState{}, err
	}
	if err := rows.Err(); err != nil {
		return upstreamIdentityState{}, err
	}
	for upstreamID := range state.identityHosts {
		sort.Strings(state.identityHosts[upstreamID])
	}
	return state, nil
}

func (state upstreamIdentityState) components() [][]string {
	components := [][]string{}
	seen := map[string]struct{}{}
	for _, start := range sortedStringMapKeys(state.graph) {
		if _, visited := seen[start]; visited {
			continue
		}
		component := []string{}
		queue := []string{start}
		seen[start] = struct{}{}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for neighbor := range state.graph[current] {
				if _, visited := seen[neighbor]; visited {
					continue
				}
				seen[neighbor] = struct{}{}
				queue = append(queue, neighbor)
			}
		}
		sort.Strings(component)
		components = append(components, component)
	}
	return components
}

func (state upstreamIdentityState) componentIndex() ([][]string, map[string][]string) {
	components := state.components()
	byHost := make(map[string][]string, len(state.graph))
	for _, component := range components {
		for _, host := range component {
			byHost[host] = component
		}
	}
	return components, byHost
}

func (state upstreamIdentityState) reconciliationComponents() [][]string {
	result := [][]string{}
	for _, component := range state.components() {
		if state.componentNeedsReconciliation(component) {
			result = append(result, component)
		}
	}
	return result
}

func (state upstreamIdentityState) componentNeedsReconciliation(component []string) bool {
	identities := map[string]struct{}{}
	missing := false
	for _, host := range component {
		mapping, found := state.existing[host]
		if !found || strings.TrimSpace(mapping.id) == "" {
			missing = true
			continue
		}
		identities[mapping.id] = struct{}{}
	}
	return missing || len(identities) != 1
}

func (state upstreamIdentityState) reconciliationHosts() []string {
	hosts := map[string]struct{}{}
	for _, component := range state.reconciliationComponents() {
		for _, host := range component {
			hosts[host] = struct{}{}
		}
	}
	return sortedStringMapKeys(hosts)
}

func (s *Store) ensureMissingUpstreamIdentities(ctx context.Context) error {
	var missing bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM upstreams u LEFT JOIN upstream_identity_hosts h ON h.host=u.host WHERE h.host IS NULL
	)`).Scan(&missing); err != nil {
		return err
	}
	if !missing {
		return nil
	}
	return s.ensureUpstreamIdentities(ctx)
}

func reconcileUpstreamIdentityComponent(
	ctx context.Context,
	tx *sql.Tx,
	hosts []string,
	storedHosts map[string]struct{},
	preferredPrimary map[string]struct{},
	existing map[string]upstreamIdentityMapping,
) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	identities := map[string]string{}
	primary := ""
	for _, host := range hosts {
		mapping, found := existing[host]
		if !found {
			continue
		}
		identities[mapping.id] = mapping.createdAt
		if mapping.primary {
			primary = host
		}
	}
	upstreamID := ""
	createdAt := now
	for candidate, candidateCreatedAt := range identities {
		if upstreamID == "" || candidateCreatedAt < createdAt || (candidateCreatedAt == createdAt && candidate < upstreamID) {
			upstreamID, createdAt = candidate, candidateCreatedAt
		}
	}
	if upstreamID == "" {
		var err error
		upstreamID, err = newUpstreamID()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES(?,?,?)`, upstreamID, now, now); err != nil {
			return err
		}
	}
	for candidate := range identities {
		if candidate == upstreamID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,parent_entity_id,name,observed_status,lifecycle_state,missing_observations,
			last_seen_at,missing_since,confirmed_missing_at,updated_at
		) SELECT ?,entity_kind,entity_id,parent_entity_id,name,observed_status,lifecycle_state,missing_observations,
			last_seen_at,missing_since,confirmed_missing_at,updated_at FROM upstream_catalog_entities WHERE upstream_id=?
			ON CONFLICT(upstream_id,entity_kind,entity_id) DO UPDATE SET
			parent_entity_id=excluded.parent_entity_id,name=excluded.name,observed_status=excluded.observed_status,
			lifecycle_state=excluded.lifecycle_state,missing_observations=excluded.missing_observations,
			last_seen_at=excluded.last_seen_at,missing_since=excluded.missing_since,
			confirmed_missing_at=excluded.confirmed_missing_at,updated_at=excluded.updated_at
			WHERE excluded.updated_at>upstream_catalog_entities.updated_at`, upstreamID, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_catalog_entities WHERE upstream_id=?`, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE binding_identities SET upstream_id=?,updated_at=? WHERE upstream_id=?`, upstreamID, now, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE onboarding_pending SET upstream_id=?,updated_at=? WHERE upstream_id=?`, upstreamID, now, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_identity_hosts SET upstream_id=?,updated_at=? WHERE upstream_id=?`, upstreamID, now, candidate); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM upstream_identities WHERE upstream_id=?`, candidate); err != nil {
			return err
		}
	}
	for _, host := range hosts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at)
			VALUES(?,?,0,?) ON CONFLICT(host) DO UPDATE SET upstream_id=excluded.upstream_id,updated_at=excluded.updated_at`, host, upstreamID, now); err != nil {
			return err
		}
	}
	for _, host := range hosts {
		if _, preferred := preferredPrimary[host]; preferred {
			primary = host
			break
		}
	}
	if primary == "" {
		for _, host := range hosts {
			if _, stored := storedHosts[host]; stored {
				primary = host
				break
			}
		}
	}
	if primary == "" && len(hosts) > 0 {
		primary = hosts[0]
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstream_identity_hosts SET is_primary=CASE WHEN host=? THEN 1 ELSE 0 END,updated_at=?
		WHERE upstream_id=?`, primary, now, upstreamID); err != nil {
		return err
	}
	return nil
}

func (s *Store) createUpstreamIdentityTx(ctx context.Context, tx *sql.Tx, host, now string) (string, error) {
	upstreamID, err := newUpstreamID()
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES(?,?,?)`, upstreamID, now, now); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES(?,?,1,?)`, canonicalHost(host), upstreamID, now); err != nil {
		return "", err
	}
	return upstreamID, nil
}

func (s *Store) upstreamIdentityID(ctx context.Context, host string) (string, error) {
	host = canonicalHost(host)
	if err := s.ensureUpstreamIdentities(ctx); err != nil {
		return "", err
	}
	return s.lookupUpstreamIdentityID(ctx, host)
}

func (s *Store) lookupUpstreamIdentityID(ctx context.Context, host string) (string, error) {
	var upstreamID string
	err := s.db.QueryRowContext(ctx, `SELECT upstream_id FROM upstream_identity_hosts WHERE host=?`, canonicalHost(host)).Scan(&upstreamID)
	return upstreamID, err
}

func upstreamIdentityHostsForQueryer(ctx context.Context, queryer upstreamIdentityQueryer, host string) (string, []string, error) {
	host = canonicalHost(host)
	var upstreamID string
	if err := queryer.QueryRowContext(ctx, `SELECT upstream_id FROM upstream_identity_hosts WHERE host=?`, host).Scan(&upstreamID); err != nil {
		return "", nil, err
	}
	rows, err := queryer.QueryContext(ctx, `SELECT host FROM upstream_identity_hosts WHERE upstream_id=? ORDER BY host`, upstreamID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	hosts := []string{}
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return "", nil, err
		}
		hosts = append(hosts, candidate)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	return upstreamID, hosts, nil
}

func upstreamIdentityHostSets(ctx context.Context, queryer upstreamIdentityQueryer) (map[string][]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT upstream_id,host FROM upstream_identity_hosts ORDER BY upstream_id,host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]string{}
	for rows.Next() {
		var upstreamID, host string
		if err := rows.Scan(&upstreamID, &host); err != nil {
			return nil, err
		}
		result[upstreamID] = append(result[upstreamID], host)
	}
	return result, rows.Err()
}

func sqlStringArguments(values []string) (string, []any) {
	arguments := make([]any, len(values))
	for index := range values {
		arguments[index] = values[index]
	}
	return strings.TrimRight(strings.Repeat("?,", len(values)), ","), arguments
}

func newUpstreamID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "up_" + hex.EncodeToString(raw), nil
}

func sortedStringMapKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
