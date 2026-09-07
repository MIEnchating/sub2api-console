package sqliteutil

import (
	"net/url"
	"path/filepath"
	"strings"
)

// DSN keeps filesystem names separate from SQLite URI options. Opaque also
// preserves relative paths instead of turning them into URI authorities.
func DSN(path, options string) string {
	path = filepath.ToSlash(path)
	escaped := strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(path)
	return (&url.URL{Scheme: "file", Opaque: escaped, RawQuery: options}).String()
}
