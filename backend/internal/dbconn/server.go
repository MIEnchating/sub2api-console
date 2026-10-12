package dbconn

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func openServer(ctx context.Context, config Config) (connection, error) {
	if config.Path != "" {
		return nil, errors.New("网络数据库不能设置 SQLite 文件路径")
	}
	var connector driver.Connector
	var err error
	if config.Driver == Postgres {
		connector, err = postgresConnector(config.DSN)
	} else {
		connector, err = mysqlConnector(config.DSN)
	}
	if err != nil {
		return nil, err
	}
	pool := sql.OpenDB(connector)
	configurePool(pool, config)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		_ = pool.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Driver errors can include the complete DSN. Never wrap them in public
		// startup errors or log them from this boundary.
		if pingCtx.Err() != nil {
			return nil, fmt.Errorf("%s 数据库连接超时，请检查地址与网络", config.Driver)
		}
		return nil, fmt.Errorf("%s 数据库连接失败，请检查地址、认证和 TLS 配置", config.Driver)
	}
	return pool, nil
}

func postgresConnector(dsn string) (driver.Connector, error) {
	invalid := errors.New("PostgreSQL 需要有效的连接 URL，显式指定地址、用户和数据库")
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.User == nil || parsed.User.Username() == "" || strings.Trim(parsed.Path, "/") == "" {
		return nil, invalid
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, invalid
	}
	// Avoid cached prepared statements. Console's instance and migration locks
	// still require a direct connection or a proxy with session affinity.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	config.ConnectTimeout = 10 * time.Second
	return stdlib.GetConnector(*config), nil
}

func mysqlConnector(dsn string) (driver.Connector, error) {
	invalid := errors.New("MySQL 需要有效的连接 DSN，显式指定地址、用户和数据库")
	if strings.TrimSpace(dsn) == "" {
		return nil, invalid
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil || config.User == "" || config.DBName == "" {
		return nil, invalid
	}
	if config.MultiStatements {
		return nil, errors.New("MySQL 不允许启用 multiStatements，请逐条执行迁移")
	}
	config.ParseTime = true
	config.Loc = time.UTC
	config.Timeout = 10 * time.Second
	// A conditional no-op update must report matched rows consistently. UPSERT
	// affected-row semantics still require operation-specific handling.
	config.ClientFoundRows = true
	// The driver otherwise logs some transport failures to stderr. Startup
	// diagnostics are emitted through the sanitized error boundary above.
	config.Logger = silentMySQLLogger{}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		return nil, invalid
	}
	return connector, nil
}

type silentMySQLLogger struct{}

func (silentMySQLLogger) Print(...any) {}
