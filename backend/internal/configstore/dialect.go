package configstore

import "github.com/MIEnchating/sub2api-console/backend/internal/dbconn"

func (s *Store) jsonRevision() string {
	switch s.db.Driver() {
	case dbconn.Postgres:
		return `CAST(value::jsonb->>'revision' AS BIGINT)`
	case dbconn.MySQL:
		return `CAST(JSON_UNQUOTE(JSON_EXTRACT(value,'$.revision')) AS SIGNED)`
	default:
		return `json_extract(value,'$.revision')`
	}
}

func (s *Store) dialect(standard, mysql string) string {
	if s.db.Driver() == dbconn.MySQL {
		return mysql
	}
	return standard
}
