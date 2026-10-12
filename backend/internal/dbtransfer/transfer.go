package dbtransfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type Options struct {
	Sources         Sources
	BackupDirectory string
	Target          dbconn.Config
	// Explicit offline acknowledgement is required by the command as well.
	Stopped bool
}

type Report struct {
	FormatVersion int           `json:"format_version"`
	Driver        dbconn.Driver `json:"driver"`
	State         string        `json:"state"`
	CompletedAt   string        `json:"completed_at"`
	Tables        []TableReport `json:"tables"`
}

func initializeStores(ctx context.Context, configs []dbconn.Config) error {
	private, err := configstore.OpenDatabase(ctx, configs[0])
	if err != nil {
		return errors.New("配置数据库结构初始化失败，请检查兼容版本或迁移状态")
	}
	if err := private.Close(); err != nil {
		return err
	}
	data, err := business.OpenDatabase(ctx, configs[1])
	if err != nil {
		return errors.New("业务数据库结构初始化失败，请检查兼容版本或迁移状态")
	}
	if err := data.Close(); err != nil {
		return err
	}
	tasks, err := taskstore.OpenDatabase(ctx, configs[2])
	if err != nil {
		return errors.New("任务数据库结构初始化失败，请检查兼容版本或迁移状态")
	}
	return tasks.Close()
}

// Transfer keeps all target data in one transaction. DDL and import marker are
// committed separately so MySQL DDL failures remain visible and block startup.
func Transfer(ctx context.Context, options Options) (Report, error) {
	report := Report{FormatVersion: 1, Driver: options.Target.Driver, State: "incomplete", Tables: []TableReport{}}
	if !options.Stopped {
		return report, errors.New("必须先停止源控制台并明确确认停写")
	}
	if options.Target.Driver != dbconn.Postgres && options.Target.Driver != dbconn.MySQL {
		return report, errors.New("迁移目标必须为 postgres 或 mysql")
	}
	db, err := dbconn.Open(ctx, options.Target)
	if err != nil {
		return report, err
	}
	defer db.Close()
	instance, err := db.AcquireInstance(ctx)
	if err != nil {
		return report, err
	}
	defer instance.Close()
	ctx = instance.Context()
	snapshot, err := SnapshotSources(ctx, options.Sources, options.BackupDirectory)
	if err != nil {
		return report, err
	}
	defer snapshot.Close()
	sourceConfigs := []dbconn.Config{{Driver: dbconn.SQLite, Path: snapshot.Prepared.Config}, {Driver: dbconn.SQLite, Path: snapshot.Prepared.Business}, {Driver: dbconn.SQLite, Path: snapshot.Prepared.Tasks}}
	if err := initializeStores(ctx, sourceConfigs); err != nil {
		return report, err
	}
	tables := []table{}
	for index, module := range []string{"config", "business", "tasks"} {
		source, err := sql.Open("sqlite", sqliteutil.DSN(sourceConfigs[index].Path, "mode=ro"))
		if err != nil {
			return report, errors.New("无法读取准备后的源库")
		}
		defer source.Close()
		items, err := sourceTables(ctx, source, module)
		if err != nil {
			return report, fmt.Errorf("%s 源库校验失败：%w", module, err)
		}
		tables = append(tables, items...)
	}
	tables, err = orderedTables(tables)
	if err != nil {
		return report, err
	}
	if err := db.BeginImport(ctx); err != nil {
		return report, err
	}
	importCtx := dbconn.ImportContext(ctx)
	if err := initializeStores(importCtx, []dbconn.Config{options.Target, options.Target, options.Target}); err != nil {
		return report, err
	}
	if err := matchTarget(ctx, db, tables); err != nil {
		return report, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return report, errors.New("无法开始数据导入事务")
	}
	defer tx.Rollback()
	if db.Driver() == dbconn.MySQL {
		if _, err := tx.ExecContext(ctx, `SET SESSION sql_mode=CONCAT(@@SESSION.sql_mode,',NO_AUTO_VALUE_ON_ZERO')`); err != nil {
			return report, errors.New("无法配置 MySQL 原始零 ID 保留模式")
		}
	}
	// The target was proved empty before schema creation. These rows are only
	// deterministic schema seeds; remove them before copying their source table.
	for index := len(tables) - 1; index >= 0; index-- {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+quoted(tables[index].name)); err != nil {
			return report, fmt.Errorf("清理目标种子表 %s 失败", tables[index].name)
		}
	}
	for _, item := range tables {
		entry, err := copyTable(ctx, tx, item)
		if err != nil {
			return report, err
		}
		report.Tables = append(report.Tables, entry)
	}
	if err := business.RebuildImportedState(ctx, tx); err != nil {
		return report, errors.New("重建业务时间索引或事件编号失败")
	}
	if err := taskstore.RebuildImportedState(ctx, tx); err != nil {
		return report, errors.New("重建任务历史索引失败")
	}
	if err := alignIdentities(ctx, tx, tables); err != nil {
		return report, errors.New("对齐目标自增序列失败")
	}
	for index, item := range tables {
		if err := verifyTable(ctx, tx, item, &report.Tables[index]); err != nil {
			return report, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE console_data_import SET state='complete' WHERE id=1 AND state='pending'`); err != nil {
		return report, errors.New("无法完成数据库导入标记")
	}
	if err := tx.Commit(); err != nil {
		return report, errors.New("导入提交结果未确认；请检查目标导入标记，不要直接重试")
	}
	report.State = "complete"
	report.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}
