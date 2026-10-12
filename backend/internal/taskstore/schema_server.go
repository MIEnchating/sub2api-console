package taskstore

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func (s *Store) ensureServerSchema(ctx context.Context) error {
	return s.db.Migrate(ctx, "tasks", []dbconn.Migration{{Version: 1, Statements: map[dbconn.Driver][]string{
		dbconn.Postgres: {
			`CREATE TABLE tasks (id TEXT COLLATE "C" PRIMARY KEY, skill TEXT COLLATE "C" NOT NULL, operation TEXT COLLATE "C" NOT NULL, status TEXT NOT NULL, progress INTEGER NOT NULL, message TEXT NOT NULL, result_json TEXT NOT NULL, created_at TEXT COLLATE "C" NOT NULL, updated_at TEXT COLLATE "C" NOT NULL)`,
			`CREATE TABLE active_task_operations (operation TEXT PRIMARY KEY, task_id TEXT NOT NULL UNIQUE)`,
			`CREATE TABLE deleted_tasks (id TEXT PRIMARY KEY, deleted_at TEXT NOT NULL)`,
			`CREATE TABLE task_recovery (task_id TEXT PRIMARY KEY, payload TEXT NOT NULL)`,
			`CREATE TABLE task_write_locks (id VARCHAR(64) PRIMARY KEY)`,
			`CREATE TABLE task_animation_history (task_id TEXT COLLATE "C" NOT NULL REFERENCES tasks(id) ON DELETE CASCADE, entry_no BIGINT NOT NULL, account_id TEXT COLLATE "C" NOT NULL, account_hash VARCHAR(64) NOT NULL, stage VARCHAR(16) NOT NULL, entry_kind VARCHAR(16) NOT NULL, completed_seconds BIGINT NOT NULL, completed_nanos BIGINT NOT NULL, PRIMARY KEY(task_id,entry_no))`,
			`CREATE INDEX ix_task_animation_account ON task_animation_history(account_hash,stage,entry_kind,completed_seconds DESC,completed_nanos DESC)`,
			`CREATE TABLE task_model_history (task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,payload TEXT NOT NULL)`,
			`CREATE TABLE task_model_accounts (task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,account_hash VARCHAR(64) NOT NULL,account_id TEXT COLLATE "C" NOT NULL,PRIMARY KEY(task_id,account_hash))`,
			`CREATE INDEX ix_tasks_updated_at ON tasks(updated_at DESC,id)`,
			`CREATE INDEX ix_tasks_status_updated_at ON tasks(status,updated_at,id)`,
			`CREATE INDEX ix_tasks_skill_updated_at ON tasks(skill,updated_at DESC,id)`,
			`CREATE INDEX ix_tasks_operation_status_updated_at ON tasks(operation,status,updated_at DESC,id DESC)`,
		},
		dbconn.MySQL: {
			`CREATE TABLE tasks (id VARBINARY(512) PRIMARY KEY, skill VARCHAR(128) NOT NULL, operation VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL, progress INTEGER NOT NULL, message LONGTEXT NOT NULL, result_json LONGTEXT NOT NULL, created_at VARCHAR(40) NOT NULL, updated_at VARCHAR(40) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE TABLE active_task_operations (operation VARCHAR(128) PRIMARY KEY, task_id VARBINARY(512) NOT NULL UNIQUE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE TABLE deleted_tasks (id VARBINARY(512) PRIMARY KEY, deleted_at VARCHAR(40) NOT NULL) ENGINE=InnoDB`,
			`CREATE TABLE task_recovery (task_id VARBINARY(512) PRIMARY KEY, payload LONGTEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE TABLE task_write_locks (id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY) ENGINE=InnoDB`,
			`CREATE TABLE task_animation_history (task_id VARBINARY(512) NOT NULL,entry_no BIGINT NOT NULL,account_id LONGTEXT NOT NULL,account_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,stage VARCHAR(16) NOT NULL,entry_kind VARCHAR(16) NOT NULL,completed_seconds BIGINT NOT NULL,completed_nanos BIGINT NOT NULL,PRIMARY KEY(task_id,entry_no),FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE INDEX ix_task_animation_account ON task_animation_history(account_hash,stage,entry_kind,completed_seconds DESC,completed_nanos DESC)`,
			`CREATE TABLE task_model_history (task_id VARBINARY(512) PRIMARY KEY,payload LONGTEXT NOT NULL,FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE TABLE task_model_accounts (task_id VARBINARY(512) NOT NULL,account_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,account_id LONGTEXT NOT NULL,PRIMARY KEY(task_id,account_hash),FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`,
			`CREATE INDEX ix_tasks_updated_at ON tasks(updated_at DESC,id)`,
			`CREATE INDEX ix_tasks_status_updated_at ON tasks(status,updated_at,id)`,
			`CREATE INDEX ix_tasks_skill_updated_at ON tasks(skill,updated_at DESC,id)`,
			`CREATE INDEX ix_tasks_operation_status_updated_at ON tasks(operation,status,updated_at DESC,id DESC)`,
		},
	}}})
}
