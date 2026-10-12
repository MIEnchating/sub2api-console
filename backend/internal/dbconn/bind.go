package dbconn

import (
	"strconv"
	"strings"
)

// Bind converts positional parameters, preserving quoted text, identifiers and
// comments. SQL authored for this API escapes a PostgreSQL JSON ? operator as ??.
func (d Driver) Bind(query string) string {
	if d == SQLite {
		return query
	}
	var out strings.Builder
	parameter := 0
	for i := 0; i < len(query); {
		start := i
		closedQuote := false
		switch query[i] {
		case '\'', '"', '`':
			quote := query[i]
			escaped := quote == '\'' && (d == MySQL || start > 0 && (query[start-1] == 'E' || query[start-1] == 'e'))
			i++
			for i < len(query) {
				if escaped && query[i] == '\\' && i+1 < len(query) {
					i += 2
					continue
				}
				if query[i] == quote {
					i++
					if i < len(query) && query[i] == quote {
						i++
						continue
					}
					closedQuote = true
					break
				}
				i++
			}
		case '-':
			if i+1 < len(query) && query[i+1] == '-' {
				for i < len(query) && query[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		case '/':
			if i+1 < len(query) && query[i+1] == '*' {
				i += 2
				depth := 1
				for i < len(query) && depth > 0 {
					if i+1 < len(query) && query[i:i+2] == "/*" {
						depth++
						i += 2
					} else if i+1 < len(query) && query[i:i+2] == "*/" {
						depth--
						i += 2
					} else {
						i++
					}
				}
			} else {
				i++
			}
		case '$':
			end := i + 1
			for end < len(query) && (query[end] == '_' || query[end] >= 'a' && query[end] <= 'z' || query[end] >= 'A' && query[end] <= 'Z' || end > i+1 && query[end] >= '0' && query[end] <= '9') {
				end++
			}
			if end < len(query) && query[end] == '$' {
				delimiter := query[i : end+1]
				if close := strings.Index(query[end+1:], delimiter); close >= 0 {
					i = end + 1 + close + len(delimiter)
				} else {
					i = len(query)
				}
			} else {
				i++
			}
		case '?':
			if d == MySQL {
				out.WriteByte('?')
				i++
				continue
			}
			if i+1 < len(query) && query[i+1] == '?' {
				out.WriteByte('?')
				i += 2
			} else {
				parameter++
				out.WriteByte('$')
				out.WriteString(strconv.Itoa(parameter))
				i++
			}
			continue
		default:
			i++
		}
		if d == MySQL && query[start] == '"' && closedQuote {
			identifier := strings.ReplaceAll(query[start+1:i-1], "\"\"", "\"")
			out.WriteByte('`')
			out.WriteString(strings.ReplaceAll(identifier, "`", "``"))
			out.WriteByte('`')
		} else {
			out.WriteString(query[start:i])
		}
	}
	return out.String()
}
