package taskstore

import (
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func (s *Store) validResultSQL() string {
	if s.db.Driver() == dbconn.Postgres {
		return `result_json IS JSON`
	}
	return `JSON_VALID(result_json)`
}

func (s *Store) objectResultSQL() string {
	if s.db.Driver() == dbconn.Postgres {
		return `result_json IS JSON OBJECT`
	}
	return `CASE WHEN JSON_VALID(result_json) THEN JSON_TYPE(result_json)='OBJECT' ELSE FALSE END`
}

// Paths here are fixed source-code constants, never caller input.
func (s *Store) resultTextSQL(keys ...string) string {
	fields := make([]string, len(keys))
	for i, key := range keys {
		if s.db.Driver() == dbconn.Postgres {
			fields[i] = `result_json::jsonb->>'` + key + `'`
		} else {
			fields[i] = `NULLIF(JSON_UNQUOTE(JSON_EXTRACT(result_json,'$.` + key + `')),'null')`
		}
	}
	expression := fields[0]
	if len(fields) > 1 {
		expression = `COALESCE(` + strings.Join(fields, ",") + `)`
	}
	return `CASE WHEN ` + s.validResultSQL() + ` THEN ` + expression + ` END`
}

// Replace only named, authored summary expressions. Other task operations have
// their own explicit dialect queries; this is not a general SQL translator.
func (s *Store) summarySQL(query string) string {
	if s.db.Driver() == dbconn.SQLite {
		return query
	}
	system := `CASE WHEN ` + s.resultTextSQL("system_info") + ` IN ('true','1') THEN 1 ELSE 0 END`
	console := `(` + system + `=1 OR operation IN ('account-model-animation','account-model-precheck','account-model-combined','managed-model-detection','account-terminal-continuity','account-model-automation','account-model-automation-group') OR (operation='active-probe' AND TRIM(COALESCE(` + s.resultTextSQL("platform") + `,''))<>'' AND TRIM(COALESCE(` + s.resultTextSQL("model") + `,''))<>''))`
	query = strings.NewReplacer(
		taskConsoleWhereSQL, console,
		taskRunKeySQL, s.resultTextSQL("run_key"),
		taskObjectSQL, s.resultTextSQL("host", "account_name", "account_id", "object_label"),
		taskRequestSQL, s.resultTextSQL("request_id", "req", "client_request_id", "client_req"),
		taskModelSQL, s.resultTextSQL("model"),
		taskErrorSQL, s.resultTextSQL("error", "detail", "summary"),
		` INDEXED BY ix_tasks_log_search`, "",
	).Replace(query)
	if s.db.Driver() == dbconn.Postgres {
		query = strings.ReplaceAll(query, "instr(", "strpos(")
	}
	return query
}
