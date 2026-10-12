// Package dbconn owns database connections, not SQL dialect translation.
// Callers must supply statements and placeholders for the selected engine.
package dbconn

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

type Driver string

const (
	Postgres Driver = "postgres"
	MySQL    Driver = "mysql"
	SQLite   Driver = "sqlite"
)

// Path is exclusively a SQLite filename; DSN is exclusively a server
// connection configuration. pgx also honors its standard PG* settings.
type Config struct {
	Driver       Driver
	DSN          string
	Path         string
	MaxOpenConns int
	MaxIdleConns int
}

type connection interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	Close() error
	Conn(context.Context) (*sql.Conn, error)
	Stats() sql.DBStats
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
}

type Database struct {
	driver Driver
	pool   connection
	config Config
}

func Open(ctx context.Context, config Config) (*Database, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if config.Driver == "" {
		config.Driver = Postgres
	}
	if config.MaxOpenConns < 0 || config.MaxIdleConns < 0 {
		return nil, errors.New("数据库连接池大小不能为负数")
	}
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 16
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = min(4, config.MaxOpenConns)
	}
	if config.MaxIdleConns > config.MaxOpenConns {
		return nil, errors.New("数据库空闲连接数不能大于最大连接数")
	}
	var pool connection
	var err error
	switch config.Driver {
	case SQLite:
		pool, err = openSQLite(config)
	case Postgres, MySQL:
		pool, err = openServer(ctx, config)
	default:
		return nil, errors.New("数据库类型必须为 postgres、mysql 或 sqlite")
	}
	if err != nil {
		return nil, err
	}
	return &Database{driver: config.Driver, pool: pool, config: config}, nil
}

func openSQLite(config Config) (connection, error) {
	if config.DSN != "" {
		return nil, errors.New("SQLite 必须使用文件路径，不能设置网络数据库 DSN")
	}
	if err := sqliteutil.Prepare(config.Path); err != nil {
		return nil, err
	}
	pool, err := sqliteutil.OpenDatabase(config.Path)
	if err != nil {
		return nil, err
	}
	if err := sqliteutil.Secure(config.Path); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return pool, nil
}

func (db *Database) Driver() Driver        { return db.driver }
func (db *Database) Stats() sql.DBStats    { return db.pool.Stats() }
func (db *Database) SetMaxOpenConns(n int) { db.pool.SetMaxOpenConns(n) }
func (db *Database) SetMaxIdleConns(n int) { db.pool.SetMaxIdleConns(n) }

// Conn is reserved for engine diagnostics and migration infrastructure. Normal
// reads and writes must use the bound query methods and transaction wrapper.
func (db *Database) Conn(ctx context.Context) (*sql.Conn, error) { return db.pool.Conn(ctx) }
func (db *Database) Close() error                                { return db.pool.Close() }
func (db *Database) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.pool.ExecContext(ctx, db.driver.Bind(query), args...)
}
func (db *Database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.pool.QueryContext(ctx, db.driver.Bind(query), args...)
}
func (db *Database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.pool.QueryRowContext(ctx, db.driver.Bind(query), args...)
}
func (db *Database) BeginTx(ctx context.Context, options *sql.TxOptions) (*Tx, error) {
	raw, err := db.pool.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &Tx{raw: raw, driver: db.driver}, nil
}

// BeginSnapshot pins the snapshot on the first table read. PostgreSQL's default
// READ COMMITTED would otherwise allow subsequent reads to observe new commits.
func (db *Database) BeginSnapshot(ctx context.Context) (*Tx, error) {
	options := &sql.TxOptions{ReadOnly: true}
	if db.driver != SQLite {
		options.Isolation = sql.LevelRepeatableRead
	}
	return db.BeginTx(ctx, options)
}

func configurePool(pool *sql.DB, config Config) {
	pool.SetMaxOpenConns(config.MaxOpenConns)
	pool.SetMaxIdleConns(config.MaxIdleConns)
	pool.SetConnMaxIdleTime(time.Minute)
	pool.SetConnMaxLifetime(30 * time.Minute)
}
