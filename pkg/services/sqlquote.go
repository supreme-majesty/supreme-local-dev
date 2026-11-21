package services

import "strings"

// quoteMy quotes a MySQL/MariaDB identifier (database, table or column name),
// doubling any backtick inside it, so names containing quotes or spaces work
// and cannot end the identifier early.
func quoteMy(ident string) string {
	return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
}

// quotePG quotes a PostgreSQL identifier, doubling any double quote inside it.
func quotePG(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

// returnsRows reports whether a statement produces a result set, looking past
// leading whitespace, comments and parentheses (so "WITH …", "(SELECT …)" and
// "-- note\nSELECT …" are recognised).
func returnsRows(query string) bool {
	s := query
	for {
		s = strings.TrimLeft(s, " \t\r\n(")
		switch {
		case strings.HasPrefix(s, "--"), strings.HasPrefix(s, "#"):
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				return false
			}
			s = s[i+1:]
		case strings.HasPrefix(s, "/*"):
			i := strings.Index(s, "*/")
			if i < 0 {
				return false
			}
			s = s[i+2:]
		default:
			word := strings.ToUpper(s)
			if i := strings.IndexAny(word, " \t\r\n(;"); i >= 0 {
				word = word[:i]
			}
			switch word {
			case "SELECT", "SHOW", "DESC", "DESCRIBE", "EXPLAIN", "WITH", "VALUES", "TABLE", "PRAGMA", "ANALYZE", "CHECK", "CHECKSUM", "OPTIMIZE", "REPAIR":
				return true
			}
			return false
		}
	}
}

// escapeLike escapes the LIKE wildcards in s, so a search for "50%" or "a_b"
// matches those characters literally. Use with ESCAPE '\'.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
