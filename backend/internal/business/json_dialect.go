package business

import "github.com/MIEnchating/sub2api-console/backend/internal/dbconn"

// Expressions and field names are fixed in source, never accepted from callers.
func jsonValidSQL(driver dbconn.Driver, column string) string {
	if driver == dbconn.Postgres {
		return `(` + column + ` IS JSON)`
	}
	return `JSON_VALID(` + column + `)`
}

func jsonObjectSQL(driver dbconn.Driver, column string) string {
	typeSQL := `json_type(` + column + `)='object'`
	if driver == dbconn.Postgres {
		typeSQL = `jsonb_typeof(` + column + `::jsonb)='object'`
	}
	if driver == dbconn.MySQL {
		typeSQL = `JSON_TYPE(` + column + `)='OBJECT'`
	}
	return `CASE WHEN ` + jsonValidSQL(driver, column) + ` THEN ` + typeSQL + ` ELSE FALSE END`
}

func jsonFieldSQL(driver dbconn.Driver, column, key string, stringOnly bool) string {
	if driver == dbconn.SQLite {
		value := `field.value`
		if stringOnly {
			value = `CASE WHEN field.type='text' THEN field.value END`
		}
		// Match Go object decoding and the server engines when legacy JSON has
		// duplicate keys. SQLite json_extract otherwise picks the first key.
		return `(SELECT ` + value + ` FROM json_each(CASE WHEN json_valid(` + column + `) THEN ` + column + ` ELSE '{}' END) field WHERE field."key"='` + key + `' ORDER BY field.id DESC LIMIT 1)`
	}
	value := `json_extract(` + column + `,'$.` + key + `')`
	typeSQL := `json_type(` + column + `,'$.` + key + `')='text'`
	if driver == dbconn.Postgres {
		value = column + `::jsonb->>'` + key + `'`
		typeSQL = `jsonb_typeof(` + column + `::jsonb->'` + key + `')='string'`
	} else if driver == dbconn.MySQL {
		extract := `JSON_EXTRACT(` + column + `,'$.` + key + `')`
		value = `CASE WHEN JSON_TYPE(` + extract + `)<>'NULL' THEN JSON_UNQUOTE(` + extract + `) END`
		typeSQL = `JSON_TYPE(` + extract + `)='STRING'`
	}
	if stringOnly {
		value = `CASE WHEN ` + typeSQL + ` THEN ` + value + ` END`
	}
	return `CASE WHEN ` + jsonValidSQL(driver, column) + ` THEN ` + value + ` END`
}

func textCastSQL(driver dbconn.Driver, column string) string {
	if driver == dbconn.MySQL {
		return `CAST(` + column + ` AS CHAR)`
	}
	return `CAST(` + column + ` AS TEXT)`
}

func alertGroupNamesSQL(driver dbconn.Driver) string {
	aggregate := `json_group_array(group_name ORDER BY group_name)`
	if driver == dbconn.Postgres {
		aggregate = `COALESCE(json_agg(group_name ORDER BY group_name)::text,'[]')`
	}
	if driver == dbconn.MySQL {
		aggregate = `COALESCE(JSON_ARRAYAGG(CONVERT(group_name USING utf8mb4)),JSON_ARRAY())`
	}
	return `(SELECT ` + aggregate + ` FROM account_groups WHERE i.object_kind='account' AND account_id=i.object_id)`
}
