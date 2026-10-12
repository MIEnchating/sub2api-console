package business

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"strings"
)

// numericIDOrder preserves SQLite's leading decimal integer ordering, including
// saturation at MaxInt64. column is always a source-code identifier, never input.
func numericIDOrder(driver dbconn.Driver, column string) string {
	switch driver {
	case dbconn.Postgres:
		digits := `ltrim(substring(` + column + ` FROM '^[0-9]+'),'0')`
		return `CASE WHEN ` + column + ` ~ '^[0-9]' THEN CASE WHEN length(` + digits + `)>19 THEN 9223372036854775807 ELSE LEAST(COALESCE(NULLIF(` + digits + `,''),'0')::numeric,9223372036854775807) END ELSE 0 END`
	case dbconn.MySQL:
		digits := `TRIM(LEADING '0' FROM REGEXP_SUBSTR(CONVERT(` + column + ` USING utf8mb4),'^[0-9]+'))`
		return `CASE WHEN CONVERT(` + column + ` USING utf8mb4) REGEXP '^[0-9]' THEN CASE WHEN LENGTH(` + digits + `)>19 THEN 9223372036854775807 ELSE LEAST(CAST(COALESCE(NULLIF(` + digits + `,''),'0') AS DECIMAL(20,0)),9223372036854775807) END ELSE 0 END`
	default:
		return `CASE WHEN ` + column + ` GLOB '[0-9]*' THEN CAST(` + column + ` AS INTEGER) ELSE 0 END`
	}
}

// JSON scopes are validated string arrays, bound as one parameter so large
// account selections do not exceed an engine's parameter limit.
func jsonScopeRows(driver dbconn.Driver) string {
	switch driver {
	case dbconn.Postgres:
		return `(SELECT value,ordinality AS ordinal FROM jsonb_array_elements_text(?::jsonb) WITH ORDINALITY)`
	case dbconn.MySQL:
		return `JSON_TABLE(?, '$[*]' COLUMNS(value LONGTEXT PATH '$', ordinal FOR ORDINALITY))`
	default:
		return `(SELECT value,CAST("key" AS INTEGER) AS ordinal FROM json_each(?))`
	}
}

func sqliteIndexHint(driver dbconn.Driver, index string) string {
	if driver == dbconn.SQLite {
		return ` INDEXED BY ` + index
	}
	return ""
}

func nullEqualSQL(driver dbconn.Driver, left, right string) string {
	switch driver {
	case dbconn.Postgres:
		return left + ` IS NOT DISTINCT FROM ` + right
	case dbconn.MySQL:
		return left + ` <=> ` + right
	default:
		return left + ` IS ` + right
	}
}

func historySearchPredicate(driver dbconn.Driver, columns ...string) string {
	clauses := make([]string, len(columns))
	for i, column := range columns {
		switch driver {
		case dbconn.Postgres:
			clauses[i] = `strpos(lower(CAST(` + column + ` AS TEXT)),?)>0`
		case dbconn.MySQL:
			clauses[i] = `instr(lower(CONVERT(` + column + ` USING utf8mb4)),?)>0`
		default:
			clauses[i] = `instr(lower(CAST(` + column + ` AS TEXT)),?)>0`
		}
	}
	return strings.Join(clauses, " OR ")
}

func instantSQL(driver dbconn.Driver, expression string) string {
	if driver == dbconn.SQLite {
		return `julianday(` + expression + `)`
	}
	return `console_time_value_v1(` + expression + `)`
}

// Binary identity columns keep exact equality; case-insensitive legacy lookup
// must explicitly decode text on MySQL, where LOWER(binary) is a no-op.
func lowerTextSQL(driver dbconn.Driver, expression string) string {
	if driver == dbconn.MySQL {
		return `LOWER(CONVERT(` + expression + ` USING utf8mb4))`
	}
	return `LOWER(` + expression + `)`
}
