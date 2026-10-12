package business_test

import (
	"database/sql"
	"math/big"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractServerTimeValuesPreserveNanosecondsAndCalendarBounds(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		} // SQLite retains its existing registered Go time functions.
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, raw := range []string{
				"1970-01-01T00:00:00Z", "1969-12-31T23:59:59.123456789Z",
				"2026-10-12T08:00:00.123456789+08:00", "2026-10-12T00:00:00.123456788Z",
				"0000-01-01T00:00:00.000000001+24:00", "9999-12-31T23:59:59.999999999-24:00",
				"2000-02-29T01:02:03,123456789123Z", "1900-02-28T23:59:59-05:30",
				"unknown", "2026-02-29T00:00:00Z", "2026-01-01T24:00:00Z", "2026-01-01T00:00:60Z", "2026-01-01T00:00:00Z\n",
			} {
				t.Run(raw, func(t *testing.T) {
					var actual sql.NullString
					if err := db.QueryRowContext(t.Context(), `SELECT console_time_value_v1(?)`, raw).Scan(&actual); err != nil {
						t.Fatal(err)
					}
					parsed, parseErr := time.Parse(time.RFC3339Nano, raw)
					if parseErr != nil {
						if actual.Valid {
							t.Fatalf("invalid date accepted: %s", actual.String)
						}
						return
					}
					expected := new(big.Rat).Add(new(big.Rat).SetInt64(parsed.Unix()), big.NewRat(int64(parsed.Nanosecond()), 1000000000))
					value, ok := new(big.Rat).SetString(actual.String)
					if !actual.Valid || !ok || value.Cmp(expected) != 0 {
						t.Fatalf("time value=%s expected=%s", actual.String, expected.RatString())
					}
				})
			}
		})
	}
}
