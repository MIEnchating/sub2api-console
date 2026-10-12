package dbtransfer

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func quoted(name string) string { return `"` + name + `"` }

type column struct {
	name, kind string
	primary    int
	binary     bool
}
type table struct {
	name    string
	columns []column
	parents []string
	source  *sql.DB
	module  string
}

func sourceTables(ctx context.Context, db *sql.DB, module string) ([]table, error) {
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return nil, errors.New("源数据库完整性检查失败")
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, err
	}
	invalid := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil || invalid {
		return nil, errors.New("源数据库存在无效关联")
	}
	rows, err = db.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		if !identifier.MatchString(name) {
			rows.Close()
			return nil, errors.New("源数据库包含不支持的表名")
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result := []table{}
	for _, name := range names {
		value := table{name: name, source: db, module: module}
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoted(name)+`)`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var field column
			var cid, notNull int
			var defaultValue any
			if err := rows.Scan(&cid, &field.name, &field.kind, &notNull, &defaultValue, &field.primary); err != nil {
				rows.Close()
				return nil, err
			}
			if !identifier.MatchString(field.name) {
				rows.Close()
				return nil, errors.New("源数据库包含不支持的列名")
			}
			value.columns = append(value.columns, field)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		rows, err = db.QueryContext(ctx, `PRAGMA foreign_key_list(`+quoted(name)+`)`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, seq int
			var parent, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &seq, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				return nil, err
			}
			if parent != name {
				value.parents = append(value.parents, parent)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		// Cost triggers require account rows even on legacy schemas without FKs.
		if name == "bindings" {
			value.parents = append(value.parents, "accounts")
		}
		// Restore the original revision after import triggers increment it.
		if name == "account_stability_version" {
			value.parents = append(value.parents, "account_stability_samples")
		}
		result = append(result, value)
	}
	return result, nil
}

func orderedTables(tables []table) ([]table, error) {
	pending := append([]table(nil), tables...)
	result := []table{}
	seen := map[string]bool{}
	for len(pending) > 0 {
		progress := false
		for index := 0; index < len(pending); {
			item := pending[index]
			if slices.ContainsFunc(item.parents, func(parent string) bool { return !seen[parent] }) {
				index++
				continue
			}
			if seen[item.name] {
				return nil, errors.New("源库包含重复的模块表名")
			}
			seen[item.name] = true
			result = append(result, item)
			pending = append(pending[:index], pending[index+1:]...)
			progress = true
		}
		if !progress {
			return nil, errors.New("源数据库关联循环或缺少父表，不能导入")
		}
	}
	return result, nil
}

func matchTarget(ctx context.Context, db *dbconn.Database, tables []table) error {
	query := `SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema=current_schema()`
	if db.Driver() == dbconn.MySQL {
		query = `SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema=DATABASE()`
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	target := map[string]map[string]string{}
	for rows.Next() {
		var table, name, kind string
		if err := rows.Scan(&table, &name, &kind); err != nil {
			rows.Close()
			return err
		}
		if target[table] == nil {
			target[table] = map[string]string{}
		}
		target[table][name] = kind
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range tables {
		for index, field := range item.columns {
			kind, found := target[item.name][field.name]
			if !found {
				return errors.New("源数据库包含目标不支持的表或列，不能丢弃导入")
			}
			item.columns[index].binary = kind == "bytea" || strings.Contains(kind, "blob") || strings.Contains(kind, "binary")
		}
	}
	return nil
}
