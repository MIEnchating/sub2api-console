package dbconn

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"regexp"
	"time"
)

// Migration requires an explicitly reviewed statement list for each engine.
type Migration struct {
	Version    int
	Statements map[Driver][]string
}

var moduleName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type migrationExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

const migrationTable = `CREATE TABLE IF NOT EXISTS console_schema_migrations (
 module VARCHAR(63) NOT NULL, version INTEGER NOT NULL, checksum VARCHAR(64) NOT NULL,
 dirty INTEGER NOT NULL, PRIMARY KEY(module,version))`

// Migrate must complete for every store before background work starts. Server
// advisory locks span metadata checks and DDL commits on a dedicated connection.
func (db *Database) Migrate(ctx context.Context, module string, migrations []Migration) error {
	if err := db.CheckImport(ctx); err != nil {
		return err
	}
	if !moduleName.MatchString(module) {
		return errors.New("数据库迁移模块名无效")
	}
	checksums := make([]string, len(migrations))
	for i, migration := range migrations {
		statements, ok := migration.Statements[db.driver]
		if migration.Version != i+1 || !ok || len(statements) == 0 {
			return fmt.Errorf("模块 %s 的迁移顺序或数据库方言不完整", module)
		}
		for _, statement := range statements {
			if statement == "" {
				return errors.New("数据库迁移语句不能为空")
			}
		}
		encoded, err := json.Marshal(statements)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(encoded)
		checksums[i] = hex.EncodeToString(sum[:])
	}
	if db.driver == SQLite {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := db.migrateWith(ctx, tx, module, migrations, checksums, false); err != nil {
			return err
		}
		return tx.Commit()
	}
	pool, ok := db.pool.(*sql.DB)
	if !ok {
		return errors.New("数据库连接池类型错误")
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if db.driver == Postgres {
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(719882046)`); err != nil {
			return err
		}
	} else {
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('console_schema_migrations',10)`).Scan(&acquired); err != nil {
			return err
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return errors.New("数据库迁移锁被占用，请稍后重试")
		}
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		query := `SELECT pg_advisory_unlock(719882046)`
		if db.driver == MySQL {
			query = `SELECT RELEASE_LOCK('console_schema_migrations')`
		}
		if _, err := conn.ExecContext(cleanup, query); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	if db.driver == Postgres {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := db.migrateWith(ctx, tx, module, migrations, checksums, false); err != nil {
			return err
		}
		return tx.Commit()
	}
	return db.migrateWith(ctx, conn, module, migrations, checksums, true)
}

func (db *Database) migrateWith(ctx context.Context, exec migrationExecutor, module string, migrations []Migration, checksums []string, nonTransactional bool) error {
	if _, err := exec.ExecContext(ctx, migrationTable); err != nil {
		return err
	}
	bind := func(pg, other string) string {
		if db.driver == Postgres {
			return pg
		}
		return other
	}
	rows, err := exec.QueryContext(ctx, bind(`SELECT version,checksum,dirty FROM console_schema_migrations WHERE module=$1 ORDER BY version`, `SELECT version,checksum,dirty FROM console_schema_migrations WHERE module=? ORDER BY version`), module)
	if err != nil {
		return err
	}
	applied := 0
	for rows.Next() {
		var version, dirty int
		var checksum string
		if err := rows.Scan(&version, &checksum, &dirty); err != nil {
			rows.Close()
			return err
		}
		if dirty != 0 {
			rows.Close()
			return fmt.Errorf("模块 %s 的迁移 %d 尚未完成，修复后才能启动", module, version)
		}
		if version != applied+1 || version > len(migrations) {
			rows.Close()
			return fmt.Errorf("模块 %s 的数据库版本不兼容，禁止降级启动", module)
		}
		if checksum != checksums[version-1] {
			rows.Close()
			return fmt.Errorf("模块 %s 的迁移 %d 校验和不符", module, version)
		}
		applied++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for i := applied; i < len(migrations); i++ {
		dirty := 0
		if nonTransactional {
			dirty = 1
		}
		_, err = exec.ExecContext(ctx, bind(`INSERT INTO console_schema_migrations(module,version,checksum,dirty) VALUES($1,$2,$3,$4)`, `INSERT INTO console_schema_migrations(module,version,checksum,dirty) VALUES(?,?,?,?)`), module, i+1, checksums[i], dirty)
		if err != nil {
			return err
		}
		for j, statement := range migrations[i].Statements[db.driver] {
			if _, err = exec.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("模块 %s 的迁移 %d 第 %d 条语句失败（%s），请检查数据库结构", module, i+1, j+1, migrationErrorCode(err))
			}
		}
		if nonTransactional {
			if _, err = exec.ExecContext(ctx, `UPDATE console_schema_migrations SET dirty=0 WHERE module=? AND version=?`, module, i+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func migrationErrorCode(err error) string {
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		return fmt.Sprintf("MySQL %d", mysqlError.Number)
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return "SQLSTATE " + postgresError.Code
	}
	return "数据库执行错误"
}
