package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"

	_ "modernc.org/sqlite"
)

type Store struct {
	path string
	db   *sql.DB
}

type RuntimeSnapshot struct {
	Available           bool
	Keys                any
	Mode                string
	ConfigurationErrors []string
}

type OverviewSummary struct {
	Available    bool
	Accounts     int
	Groups       int
	Alerts       int
	Runs         int
	LastActivity *string
}

type policyQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type policyNode struct {
	id          int64
	parentID    sql.NullInt64
	keyName     sql.NullString
	listIndex   sql.NullInt64
	nodeType    string
	scalarValue sql.NullString
}

func Open(path string) (*Store, error) {
	if err := sqliteutil.Prepare(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_txlock=immediate&_pragma=busy_timeout%2810000%29&_pragma=journal_mode%28WAL%29&_pragma=foreign_keys%28ON%29")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	store := &Store{path: path, db: db}
	if err := db.PingContext(context.Background()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := sqliteutil.Secure(path); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ready(ctx context.Context) (bool, error) {
	var marker int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM app_state WHERE key='config'`).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Mode(ctx context.Context) (string, error) {
	snapshot, err := s.RuntimeSnapshot(ctx)
	if err != nil {
		return "", err
	}
	return snapshot.Mode, nil
}

func (s *Store) RuntimeSnapshot(ctx context.Context) (RuntimeSnapshot, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key='config'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeSnapshot{Available: false, Keys: []string{}, Mode: "未初始化", ConfigurationErrors: []string{}}, nil
	}
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return RuntimeSnapshot{Available: false, Keys: []string{}, Mode: "配置错误", ConfigurationErrors: []string{"config"}}, nil
	}
	keys, keysPresent := value["keys"]
	if !keysPresent {
		keys = []string{}
	}
	errorsFound := make([]string, 0, 2)
	if _, ok := keys.([]any); keysPresent && !ok {
		errorsFound = append(errorsFound, "keys")
	}
	mode := "配置错误"
	if rawMode, present := value["mode"]; present {
		if parsedMode, ok := rawMode.(string); ok && validMode(parsedMode) {
			mode = parsedMode
		} else {
			errorsFound = append(errorsFound, "mode")
		}
	} else {
		errorsFound = append(errorsFound, "mode")
	}
	return RuntimeSnapshot{Available: true, Keys: keys, Mode: mode, ConfigurationErrors: errorsFound}, nil
}

func (s *Store) SetMode(ctx context.Context, mode string) (RuntimeSnapshot, error) {
	if !validMode(mode) {
		return RuntimeSnapshot{}, errors.New("运行模式只能是监控模式、调度模式或完全模式")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := updateRuntimeModeTx(ctx, tx, mode, now); err != nil {
		return RuntimeSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return RuntimeSnapshot{}, err
	}
	return s.RuntimeSnapshot(ctx)
}

func updateRuntimeModeTx(ctx context.Context, tx *sql.Tx, mode string, now string) (bool, error) {
	if !validMode(mode) {
		return false, errors.New("运行模式只能是监控模式、调度模式或完全模式")
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key='config'`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	value := make(map[string]any)
	if err == nil {
		if decodeErr := json.Unmarshal([]byte(raw), &value); decodeErr != nil || value == nil {
			return false, errors.New("运行配置记录损坏，无法切换模式；请先修复 Console 配置")
		}
	}
	if keys, present := value["keys"]; present {
		if _, ok := keys.([]any); !ok {
			return false, errors.New("运行配置字段 keys 无效，无法切换模式；请先修复 Console 配置")
		}
	}
	if previous, _ := value["mode"].(string); previous == mode {
		return false, nil
	}
	value["mode"] = mode
	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO app_state(key,value_json,updated_at) VALUES('config',?,?)
		 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
		string(encoded), now,
	); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}',?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`, now); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) OverviewSummary(ctx context.Context) (OverviewSummary, error) {
	ready, err := s.Ready(ctx)
	if err != nil {
		return OverviewSummary{}, err
	}
	if !ready {
		return OverviewSummary{Available: false}, nil
	}
	var result OverviewSummary
	var lastActivity sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM accounts),
		(SELECT COUNT(*) FROM local_groups),
		(SELECT COUNT(*) FROM alert_incidents WHERE status='firing'),
		(SELECT MIN(COUNT(*),100) FROM runtime_events),
		(SELECT MAX(created_at) FROM runtime_events)`).Scan(
		&result.Accounts, &result.Groups, &result.Alerts, &result.Runs, &lastActivity,
	)
	if err != nil {
		return OverviewSummary{}, err
	}
	result.Available = true
	if lastActivity.Valid {
		canonical := lastActivity.String
		if parsed, parseErr := time.Parse(time.RFC3339Nano, lastActivity.String); parseErr == nil {
			canonical = parsed.UTC().Format(time.RFC3339Nano)
		}
		result.LastActivity = &canonical
	}
	return result, nil
}

func (s *Store) Bootstrap(ctx context.Context) error {
	ready, err := s.Ready(ctx)
	if err != nil {
		return err
	}
	if ready {
		return nil
	}
	return s.bootstrapFresh(ctx)
}

func (s *Store) EnableNotificationChannel(ctx context.Context, channelType string) error {
	normalizedType := strings.ToLower(strings.TrimSpace(channelType))
	if normalizedType == "" {
		return errors.New("通知渠道类型不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return err
	}
	if control != nil {
		rawNotifications, present := control["notifications"]
		if present && rawNotifications == nil {
			return errors.New("控制面通知配置为显式空值，无法启用通知渠道")
		}
		patched, err := patchNotificationRules(rawNotifications, normalizedType)
		if err != nil {
			return err
		}
		control["notifications"] = patched
		if err := s.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
			return err
		}
	}
	var namespace string
	var rawSnapshot string
	err = tx.QueryRowContext(
		ctx,
		`SELECT namespace,value_json FROM operational_snapshots
		 WHERE state_key='sub2api-notify-rules.json' ORDER BY updated_at DESC LIMIT 1`,
	).Scan(&namespace, &rawSnapshot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		snapshot, decodeErr := decodeJSONObject(rawSnapshot)
		if decodeErr != nil {
			return errors.New("通知规则快照损坏，无法启用通知渠道")
		}
		patched, patchErr := patchNotificationRules(snapshot, normalizedType)
		if patchErr != nil {
			return patchErr
		}
		encoded, encodeErr := json.Marshal(patched)
		if encodeErr != nil {
			return encodeErr
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE operational_snapshots SET value_json=?,updated_at=?
			 WHERE namespace=? AND state_key='sub2api-notify-rules.json'`,
			string(encoded), now, namespace,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetProbeEnabled(ctx context.Context, enabled bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return err
	}
	if control == nil {
		return errors.New("主动探测策略配置无效")
	}
	probe := make(map[string]any)
	if raw, present := control["probe"]; present {
		current, ok := raw.(map[string]any)
		if !ok {
			return errors.New("主动探测策略配置无效")
		}
		for key, value := range current {
			probe[key] = value
		}
	}
	probe["enabled"] = enabled
	control["probe"] = probe
	if err := s.writePolicyDocument(
		ctx,
		tx,
		"control-plane",
		control,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ProbeEnabled(ctx context.Context) (bool, error) {
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return false, err
	}
	if control == nil {
		return false, errors.New("主动探测策略不存在")
	}
	probe, ok := control["probe"].(map[string]any)
	if !ok {
		return false, errors.New("主动探测策略配置无效")
	}
	raw, present := probe["enabled"]
	if !present {
		return true, nil
	}
	enabled, ok := raw.(bool)
	if !ok {
		return false, errors.New("主动探测开关配置无效")
	}
	return enabled, nil
}

func patchNotificationRules(raw any, channelType string) (map[string]any, error) {
	rules := make(map[string]any)
	if raw != nil {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("通知规则配置损坏，无法启用通知渠道")
		}
		for key, item := range value {
			rules[key] = item
		}
	}
	rawChannels, present := rules["channels"]
	if !present {
		rawChannels = []any{}
	}
	channels, ok := rawChannels.([]any)
	if !ok {
		return nil, errors.New("通知规则 channels 必须是数组")
	}
	updatedChannels := make([]any, 0, len(channels)+1)
	found := false
	for _, rawChannel := range channels {
		channel, ok := rawChannel.(map[string]any)
		if !ok {
			return nil, errors.New("通知规则 channels 项必须是对象")
		}
		copy := make(map[string]any, len(channel)+1)
		for key, value := range channel {
			copy[key] = value
		}
		if rawType, present := copy["type"]; present && rawType != nil && strings.EqualFold(strings.TrimSpace(fmt.Sprint(rawType)), channelType) && !found {
			copy["type"] = channelType
			copy["enabled"] = true
			found = true
		}
		updatedChannels = append(updatedChannels, copy)
	}
	if !found {
		updatedChannels = append(updatedChannels, map[string]any{"type": channelType, "enabled": true})
	}
	rules["enabled"] = true
	rules["channels"] = updatedChannels
	return rules, nil
}

func (s *Store) readPolicyDocument(ctx context.Context, queryer policyQueryer, policyKey string) (map[string]any, error) {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT id,parent_id,key_name,list_index,node_type,scalar_value
		 FROM policy_nodes WHERE policy_key=? ORDER BY id`,
		policyKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := make(map[int64]policyNode)
	children := make(map[int64][]int64)
	rootIDs := make([]int64, 0, 1)
	for rows.Next() {
		var node policyNode
		if err := rows.Scan(&node.id, &node.parentID, &node.keyName, &node.listIndex, &node.nodeType, &node.scalarValue); err != nil {
			return nil, err
		}
		nodes[node.id] = node
		if node.parentID.Valid {
			children[node.parentID.Int64] = append(children[node.parentID.Int64], node.id)
		} else {
			rootIDs = append(rootIDs, node.id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	if len(rootIDs) != 1 {
		return nil, fmt.Errorf("策略 %s 根节点数量无效", policyKey)
	}
	decoded, err := decodePolicyNode(rootIDs[0], nodes, children)
	if err != nil {
		return nil, err
	}
	result, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("策略 %s 根节点必须是对象", policyKey)
	}
	return result, nil
}

func decodePolicyNode(id int64, nodes map[int64]policyNode, children map[int64][]int64) (any, error) {
	node, found := nodes[id]
	if !found {
		return nil, errors.New("策略节点不存在")
	}
	childIDs := append([]int64{}, children[id]...)
	switch node.nodeType {
	case "object":
		result := make(map[string]any, len(childIDs))
		sort.Slice(childIDs, func(i, j int) bool {
			return nodes[childIDs[i]].keyName.String < nodes[childIDs[j]].keyName.String
		})
		for _, childID := range childIDs {
			child := nodes[childID]
			if !child.keyName.Valid || child.listIndex.Valid {
				return nil, errors.New("策略对象子节点结构无效")
			}
			value, err := decodePolicyNode(childID, nodes, children)
			if err != nil {
				return nil, err
			}
			if _, duplicate := result[child.keyName.String]; duplicate {
				return nil, errors.New("策略对象包含重复字段")
			}
			result[child.keyName.String] = value
		}
		return result, nil
	case "array":
		sort.Slice(childIDs, func(i, j int) bool {
			return nodes[childIDs[i]].listIndex.Int64 < nodes[childIDs[j]].listIndex.Int64
		})
		result := make([]any, 0, len(childIDs))
		for index, childID := range childIDs {
			child := nodes[childID]
			if child.keyName.Valid || !child.listIndex.Valid || child.listIndex.Int64 != int64(index) {
				return nil, errors.New("策略数组子节点结构无效")
			}
			value, err := decodePolicyNode(childID, nodes, children)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		return result, nil
	case "string":
		return node.scalarValue.String, nil
	case "integer":
		return strconv.ParseInt(node.scalarValue.String, 10, 64)
	case "real":
		value, err := strconv.ParseFloat(node.scalarValue.String, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("策略包含无效实数")
		}
		return value, nil
	case "boolean":
		if node.scalarValue.String == "1" {
			return true, nil
		}
		if node.scalarValue.String == "0" {
			return false, nil
		}
		return nil, errors.New("策略包含无效布尔值")
	case "null":
		return nil, nil
	default:
		return nil, fmt.Errorf("策略包含未知节点类型：%s", node.nodeType)
	}
}

func (s *Store) writePolicyDocument(ctx context.Context, tx *sql.Tx, policyKey string, value map[string]any, updatedAt string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM policy_nodes WHERE policy_key=?`, policyKey); err != nil {
		return err
	}
	if _, err := insertPolicyNode(ctx, tx, policyKey, value, nil, nil, nil, updatedAt); err != nil {
		return err
	}
	return nil
}

func insertPolicyNode(ctx context.Context, tx *sql.Tx, policyKey string, value any, parentID *int64, keyName *string, listIndex *int64, updatedAt string) (int64, error) {
	nodeType := ""
	var scalar any
	switch item := value.(type) {
	case map[string]any:
		nodeType = "object"
	case []any:
		nodeType = "array"
	case nil:
		nodeType = "null"
	case bool:
		nodeType = "boolean"
		if item {
			scalar = "1"
		} else {
			scalar = "0"
		}
	case string:
		nodeType, scalar = "string", item
	case int:
		nodeType, scalar = "integer", strconv.Itoa(item)
	case int64:
		nodeType, scalar = "integer", strconv.FormatInt(item, 10)
	case float64:
		if math.IsNaN(item) || math.IsInf(item, 0) {
			return 0, errors.New("策略包含非有限数值")
		}
		nodeType, scalar = "real", strconv.FormatFloat(item, 'g', -1, 64)
	default:
		return 0, fmt.Errorf("策略包含不支持的值类型：%T", value)
	}
	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO policy_nodes(policy_key,parent_id,key_name,list_index,node_type,scalar_value,updated_at)
		 VALUES(?,?,?,?,?,?,?)`,
		policyKey, parentID, keyName, listIndex, nodeType, scalar, updatedAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	switch item := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childKey := key
			if _, err := insertPolicyNode(ctx, tx, policyKey, item[key], &id, &childKey, nil, updatedAt); err != nil {
				return 0, err
			}
		}
	case []any:
		for index, child := range item {
			childIndex := int64(index)
			if _, err := insertPolicyNode(ctx, tx, policyKey, child, &id, nil, &childIndex, updatedAt); err != nil {
				return 0, err
			}
		}
	}
	return id, nil
}

func decodeJSONObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	normalized, err := normalizeJSONNumbers(value)
	if err != nil {
		return nil, err
	}
	result, ok := normalized.(map[string]any)
	if !ok {
		return nil, errors.New("JSON 根节点必须是对象")
	}
	return result, nil
}

func normalizeJSONNumbers(value any) (any, error) {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			normalized, err := normalizeJSONNumbers(child)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			normalized, err := normalizeJSONNumbers(child)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	case json.Number:
		if integer, err := strconv.ParseInt(string(item), 10, 64); err == nil {
			return integer, nil
		}
		real, err := strconv.ParseFloat(string(item), 64)
		if err != nil || math.IsNaN(real) || math.IsInf(real, 0) {
			return nil, errors.New("JSON 包含非有限数值")
		}
		return real, nil
	default:
		return value, nil
	}
}

func validMode(mode string) bool {
	return runtimepolicy.Valid(mode)
}
