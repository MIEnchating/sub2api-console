package config

import (
	"errors"
	"os"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func databaseEnv() (dbconn.Config, error) {
	driver := dbconn.Driver(strings.ToLower(strings.TrimSpace(envOrDefault("SUB2API_CONSOLE_DB_DRIVER", "postgres"))))
	if driver != dbconn.Postgres && driver != dbconn.MySQL && driver != dbconn.SQLite {
		return dbconn.Config{}, errors.New("SUB2API_CONSOLE_DB_DRIVER 必须为 postgres、mysql 或 sqlite")
	}
	dsn := strings.TrimSpace(os.Getenv("SUB2API_CONSOLE_DB_DSN"))
	if driver == dbconn.SQLite && dsn != "" {
		return dbconn.Config{}, errors.New("SQLite 不能设置 SUB2API_CONSOLE_DB_DSN；请使用三个文件路径")
	}
	if driver != dbconn.SQLite && dsn == "" {
		return dbconn.Config{}, errors.New("默认使用 PostgreSQL；请设置 SUB2API_CONSOLE_DB_DSN，使用旧 SQLite 时必须显式设置 SUB2API_CONSOLE_DB_DRIVER=sqlite")
	}
	if driver != dbconn.SQLite {
		for _, name := range []string{"SUB2API_CONSOLE_TASK_DB", "SUB2API_CONSOLE_CONFIG_DB", "SUB2API_CONSOLE_DATA_DB"} {
			if _, present := os.LookupEnv(name); present {
				return dbconn.Config{}, errors.New("服务器数据库不能同时配置 SQLite 文件路径变量；旧数据须显式迁移")
			}
		}
	}
	maximum, err := positiveEnv("SUB2API_CONSOLE_DB_MAX_OPEN_CONNS", 16)
	if err != nil {
		return dbconn.Config{}, err
	}
	idle, err := positiveEnv("SUB2API_CONSOLE_DB_MAX_IDLE_CONNS", min(4, maximum))
	if err != nil {
		return dbconn.Config{}, err
	}
	if idle > maximum {
		return dbconn.Config{}, errors.New("数据库空闲连接数不能超过最大连接数")
	}
	return dbconn.Config{Driver: driver, DSN: dsn, MaxOpenConns: maximum, MaxIdleConns: idle}, nil
}

func (c Config) DatabaseFor(sqlitePath string) dbconn.Config {
	value := c.Database
	if value.Driver == dbconn.SQLite {
		value.Path = sqlitePath
	}
	return value
}

// Existing local databases must not silently become an empty server database.
// An explicit transfer leaves a completed import marker checked at startup.
func (c Config) HasLegacyDatabase() (bool, error) {
	for _, path := range []string{c.ConfigDB, c.DataDB, c.TaskDB} {
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, errors.New("无法检查旧 SQLite 数据文件")
		}
		if !info.Mode().IsRegular() {
			return false, errors.New("旧 SQLite 数据路径不是普通文件")
		}
		if info.Size() > 0 {
			return true, nil
		}
	}
	return false, nil
}
