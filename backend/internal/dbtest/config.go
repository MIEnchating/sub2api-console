package dbtest

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Only the isolated contract-test launcher may opt into network databases.
// Construct DSNs from fixed loopback endpoints: never accept a production DSN.
func Configs(t *testing.T) []dbconn.Config {
	t.Helper()
	result := []dbconn.Config{{Driver: dbconn.SQLite, Path: filepath.Join(t.TempDir(), "contract.sqlite3")}}
	if os.Getenv("CONSOLE_DB_CONTRACT_TEST") != "1" {
		return result
	}
	password := os.Getenv("CONSOLE_DB_CONTRACT_PASSWORD")
	if password == "" {
		t.Fatal("contract test password must be explicitly configured")
	}
	pgURL := &url.URL{Scheme: "postgres", User: url.UserPassword("console_contract", password), Host: "127.0.0.1:25432", Path: "console_contract_test", RawQuery: "sslmode=disable"}
	return append(result,
		dbconn.Config{Driver: dbconn.Postgres, DSN: pgURL.String()},
		dbconn.Config{Driver: dbconn.MySQL, DSN: "console_contract:" + password + "@tcp(127.0.0.1:23306)/console_contract_test"},
	)
}
