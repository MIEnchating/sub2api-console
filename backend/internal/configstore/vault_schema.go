package configstore

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Version 2 separates the immutable identity from the editable display name.
// Unresolved old selections receive a tombstone ID so a later same-name entry
// cannot silently inherit them. Version 1 server statements remain unchanged.
func vaultIdentityMigration() dbconn.Migration {
	return dbconn.Migration{Version: 2, Statements: map[dbconn.Driver][]string{
		dbconn.Postgres: {
			`ALTER TABLE vault_entries ADD COLUMN id VARCHAR(64) COLLATE "C"`,
			`UPDATE vault_entries SET id='vault_' || replace(gen_random_uuid()::text,'-','')`,
			`ALTER TABLE vault_entries ALTER COLUMN id SET NOT NULL`,
			`ALTER TABLE vault_entries DROP CONSTRAINT vault_entries_pkey`,
			`ALTER TABLE vault_entries ADD PRIMARY KEY(id)`,
			`ALTER TABLE vault_entries ADD CONSTRAINT vault_entries_name_unique UNIQUE(entry)`,
			`ALTER TABLE auth_recovery_preferences ADD COLUMN vault_entry_id VARCHAR(64) COLLATE "C"`,
			`UPDATE auth_recovery_preferences p SET vault_entry_id=COALESCE((SELECT v.id FROM vault_entries v WHERE v.entry=p.vault_entry),'missing_' || replace(gen_random_uuid()::text,'-','')) WHERE vault_entry IS NOT NULL`,
			`ALTER TABLE auth_recovery_preferences DROP COLUMN vault_entry`,
		},
		dbconn.MySQL: {
			`ALTER TABLE vault_entries ADD COLUMN id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin`,
			`UPDATE vault_entries SET id=CONCAT('vault_',REPLACE(UUID(),'-',''))`,
			`ALTER TABLE vault_entries MODIFY id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL, DROP PRIMARY KEY, ADD PRIMARY KEY(id), ADD CONSTRAINT vault_entries_name_unique UNIQUE(entry)`,
			`ALTER TABLE auth_recovery_preferences ADD COLUMN vault_entry_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin`,
			`UPDATE auth_recovery_preferences p LEFT JOIN vault_entries v ON v.entry=p.vault_entry SET p.vault_entry_id=COALESCE(v.id,CONCAT('missing_',REPLACE(UUID(),'-',''))) WHERE p.vault_entry IS NOT NULL`,
			`ALTER TABLE auth_recovery_preferences DROP COLUMN vault_entry`,
		},
	}}
}

func migrateSQLiteVaultIdentity(ctx context.Context, tx *dbconn.Tx) error {
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= 2 {
		return nil
	}
	statements := []string{
		`CREATE TABLE vault_entries_v2(id TEXT PRIMARY KEY NOT NULL,entry TEXT NOT NULL UNIQUE,username TEXT,password TEXT,hosts_json TEXT NOT NULL DEFAULT '[]',headers_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL)`,
		`INSERT INTO vault_entries_v2 SELECT 'vault_' || lower(hex(randomblob(16))),entry,username,password,hosts_json,headers_json,updated_at FROM vault_entries`,
		`ALTER TABLE auth_recovery_preferences ADD COLUMN vault_entry_id TEXT`,
		`UPDATE auth_recovery_preferences SET vault_entry_id=COALESCE((SELECT id FROM vault_entries_v2 WHERE entry=auth_recovery_preferences.vault_entry),'missing_' || lower(hex(randomblob(16)))) WHERE vault_entry IS NOT NULL`,
		`ALTER TABLE auth_recovery_preferences DROP COLUMN vault_entry`,
		`DROP TABLE vault_entries`,
		`ALTER TABLE vault_entries_v2 RENAME TO vault_entries`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
