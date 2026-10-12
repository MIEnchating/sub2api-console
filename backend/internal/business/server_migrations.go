package business

import "github.com/MIEnchating/sub2api-console/backend/internal/dbconn"

func businessServerMigrations() []dbconn.Migration {
	return []dbconn.Migration{
		{Version: 1, Statements: map[dbconn.Driver][]string{dbconn.Postgres: businessPostgresSchema, dbconn.MySQL: businessMySQLSchema}},
		{Version: 2, Statements: map[dbconn.Driver][]string{
			dbconn.Postgres: {`CREATE TABLE upstream_exclusions(upstream_id TEXT COLLATE "C" PRIMARY KEY,updated_at TEXT COLLATE "C" NOT NULL,FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE CASCADE)`},
			dbconn.MySQL:    {`CREATE TABLE upstream_exclusions(upstream_id VARBINARY(512) PRIMARY KEY,updated_at VARCHAR(40) NOT NULL,FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`},
		}},
	}
}
