package dbtransfer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"slices"
	"strconv"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

type TableReport struct {
	Module string `json:"module"`
	Table  string `json:"table"`
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

func tableColumns(item table) string {
	names := make([]string, len(item.columns))
	for index, field := range item.columns {
		names[index] = quoted(field.name)
	}
	return strings.Join(names, ",")
}

func tableOrder(item table) (string, error) {
	keys := []column{}
	for _, field := range item.columns {
		if field.primary > 0 {
			keys = append(keys, field)
		}
	}
	if len(keys) == 0 {
		return "", errors.New("源表缺少稳定主键，不能验证导入")
	}
	slices.SortFunc(keys, func(a, b column) int { return a.primary - b.primary })
	names := []string{}
	for _, key := range keys {
		names = append(names, quoted(key.name))
	}
	return strings.Join(names, ","), nil
}

func valuesFrom(rows *sql.Rows, count int) ([]any, error) {
	values := make([]any, count)
	dest := make([]any, count)
	for index := range values {
		dest[index] = &values[index]
	}
	err := rows.Scan(dest...)
	return values, err
}

func hashValues(digest hash.Hash, values []any) error {
	for _, value := range values {
		var raw []byte
		marker := byte(1)
		switch typed := value.(type) {
		case nil:
			marker = 0
		case string:
			raw = []byte(typed)
		case []byte:
			raw = typed
		case int64:
			raw = strconv.AppendInt(nil, typed, 10)
		case float64:
			raw = strconv.AppendFloat(nil, typed, 'g', -1, 64)
		default:
			return errors.New("源数据库包含不支持的存储类型")
		}
		_, _ = digest.Write([]byte{marker})
		_, _ = digest.Write(binary.BigEndian.AppendUint64(nil, uint64(len(raw))))
		_, _ = digest.Write(raw)
	}
	return nil
}

func copyTable(ctx context.Context, tx *dbconn.Tx, item table) (TableReport, error) {
	report := TableReport{Module: item.module, Table: item.name}
	order, err := tableOrder(item)
	if err != nil {
		return report, err
	}
	columns := tableColumns(item)
	selectColumns := columns
	insertColumns := columns
	// These directories historically use SQLite rowid ordering rather than IDs.
	if item.name == "local_groups" || item.name == "account_workbench_documents" {
		selectColumns += `,rowid`
		insertColumns += `,insertion_order`
	}
	query := `SELECT ` + selectColumns + ` FROM ` + quoted(item.name) + ` ORDER BY ` + order
	if item.name == "policy_nodes" {
		// Parents can have larger IDs than children in a valid historical tree.
		query = `WITH RECURSIVE hierarchy(id,depth) AS (SELECT id,0 FROM policy_nodes WHERE parent_id IS NULL UNION ALL SELECT p.id,h.depth+1 FROM policy_nodes p JOIN hierarchy h ON p.parent_id=h.id) SELECT ` + columns + ` FROM policy_nodes ORDER BY COALESCE((SELECT depth FROM hierarchy WHERE hierarchy.id=policy_nodes.id),0),id`
	}
	rows, err := item.source.QueryContext(ctx, query)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	count := len(item.columns)
	if selectColumns != columns {
		count++
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", count), ",")
	insert := `INSERT INTO ` + quoted(item.name) + `(` + insertColumns + `)`
	if tx.Driver() == dbconn.Postgres && item.name == "account_workbench_documents" {
		// Preserve legacy rowid order in the GENERATED ALWAYS identity column.
		insert += ` OVERRIDING SYSTEM VALUE`
	}
	insert += ` VALUES(` + placeholders + `)`
	for rows.Next() {
		values, err := valuesFrom(rows, count)
		if err != nil {
			return report, err
		}
		for index, field := range item.columns {
			if field.binary {
				if value, ok := values[index].(string); ok {
					values[index] = []byte(value)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, insert, values...); err != nil {
			return report, fmt.Errorf("表 %s 第 %d 行无法导入（类型、长度、唯一性或关联不兼容）；未输出字段内容", item.name, report.Rows+1)
		}
		report.Rows++
	}
	if err := rows.Err(); err != nil {
		return report, err
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	return report, nil
}

func verifyTable(ctx context.Context, tx *dbconn.Tx, item table, report *TableReport) error {
	order, err := tableOrder(item)
	if err != nil {
		return err
	}
	query := `SELECT ` + tableColumns(item) + ` FROM ` + quoted(item.name) + ` ORDER BY ` + order
	source, err := item.source.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer target.Close()
	sourceHash, targetHash := sha256.New(), sha256.New()
	var count int64
	for source.Next() {
		if !target.Next() {
			return fmt.Errorf("表 %s 的目标行数不符", item.name)
		}
		left, err := valuesFrom(source, len(item.columns))
		if err != nil {
			return err
		}
		right, err := valuesFrom(target, len(item.columns))
		if err != nil {
			return err
		}
		if err := hashValues(sourceHash, left); err != nil {
			return err
		}
		if err := hashValues(targetHash, right); err != nil {
			return err
		}
		count++
	}
	if target.Next() || count != report.Rows {
		return fmt.Errorf("表 %s 的导入行数不符", item.name)
	}
	if err := errors.Join(source.Err(), target.Err()); err != nil {
		return err
	}
	left, right := hex.EncodeToString(sourceHash.Sum(nil)), hex.EncodeToString(targetHash.Sum(nil))
	if left != right {
		return fmt.Errorf("表 %s 的内容摘要不符，目标保持不可启动", item.name)
	}
	report.SHA256 = left
	return nil
}

func alignIdentities(ctx context.Context, tx *dbconn.Tx, tables []table) error {
	if tx.Driver() != dbconn.Postgres {
		return nil
	} // MySQL advances AUTO_INCREMENT on explicit IDs.
	for _, item := range tables {
		for _, name := range []string{"id", "insertion_order"} {
			present := slices.ContainsFunc(item.columns, func(field column) bool { return field.name == name })
			if name == "insertion_order" {
				present = item.name == "local_groups" || item.name == "account_workbench_documents"
			}
			if !present {
				continue
			}
			var sequence sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT pg_get_serial_sequence(?,?)`, item.name, name).Scan(&sequence); err != nil {
				return err
			}
			if !sequence.Valid {
				continue
			}
			if _, err := tx.ExecContext(ctx, `SELECT setval(?::regclass,GREATEST(COALESCE(MAX(`+quoted(name)+`),0),1),COALESCE(MAX(`+quoted(name)+`),0)>=1) FROM `+quoted(item.name), sequence.String); err != nil {
				return err
			}
		}
	}
	return nil
}
