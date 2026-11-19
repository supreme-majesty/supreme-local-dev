package services

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
)

// openMaybeGzip opens a plain .sql file or transparently decompresses a gzipped .sql.gz file.
func openMaybeGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// Peek magic bytes (0x1f 0x8b) so both .sql.gz and misnamed gzip files work seamlessly
	br := bufio.NewReader(f)
	magic, err := br.Peek(2)
	if err == nil && len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to read gzip SQL file: %w", err)
		}
		return &multiCloser{Reader: gz, closers: []io.Closer{gz, f}}, nil
	}

	return &multiCloser{Reader: br, closers: []io.Closer{f}}, nil
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for _, c := range m.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// buildMySQLCLIArgs constructs mysql/mysqldump CLI arguments and environment for cfg.
// Password is passed via MYSQL_PWD rather than CLI flags to avoid exposing secrets or escaping bugs.
func buildMySQLCLIArgs(cfg ConnectionConfig, extraArgs ...string) (args []string, env []string) {
	user := cfg.User
	if user == "" {
		user = MySQLUser()
	}
	host := cfg.Host
	port := cfg.Port
	pass := cfg.Password

	// If no explicit target config was supplied, fall back to environment defaults
	if cfg.Host == "" && cfg.Port == "" && cfg.User == "" && cfg.Socket == "" {
		host = MySQLHost()
		port = MySQLPort()
		pass = MySQLPassword()
	}

	if cfg.Socket != "" {
		args = append(args, "-S", cfg.Socket)
	} else {
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "3306"
		}
		args = append(args, "--protocol=TCP", "-h", host, "-P", port)
	}
	args = append(args, "-u", user)
	args = append(args, extraArgs...)

	env = append([]string{}, os.Environ()...)
	if pass != "" {
		env = append(env, "MYSQL_PWD="+pass)
	}
	return args, env
}

// buildPostgresCLIArgs constructs psql/pg_dump CLI arguments and environment for cfg.
func buildPostgresCLIArgs(cfg ConnectionConfig, extraArgs ...string) (args []string, env []string) {
	user := cfg.User
	if user == "" {
		user = PostgresUser()
	}
	host := cfg.Host
	port := cfg.Port
	pass := cfg.Password

	if cfg.Host == "" && cfg.Port == "" && cfg.User == "" {
		host = PostgresHost()
		port = PostgresPort()
		pass = PostgresPassword()
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "5432"
	}

	args = append(args, "-h", host, "-p", port, "-U", user)
	args = append(args, extraArgs...)

	env = append([]string{}, os.Environ()...)
	if pass != "" {
		env = append(env, "PGPASSWORD="+pass)
	}
	return args, env
}

// splitSQLStatements splits a SQL dump stream into individual executable statements.
// Respects single/double/backtick quotes, backslash escapes, -- / # / /* */ comments, and DELIMITER changes.
func splitSQLStatements(r io.Reader) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	src := string(data)
	// Strip UTF-8 BOM if present
	src = strings.TrimPrefix(src, "\uFEFF")

	var stmts []string
	var cur strings.Builder

	delimiter := ";"
	inSingle := false
	inDouble := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	i := 0
	n := len(src)
	atLineStart := true

	for i < n {
		ch := src[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				atLineStart = true
			}
			i++
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < n && src[i+1] == '/' {
				inBlockComment = false
				i += 2
				continue
			}
			i++
			continue
		}

		// Handle DELIMITER directive at line start when outside quotes
		if atLineStart && !inSingle && !inDouble && !inBacktick {
			// Skip leading spaces/tabs on the line
			j := i
			for j < n && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r') {
				j++
			}
			if j+9 <= n && strings.EqualFold(src[j:j+9], "DELIMITER") && (j+9 == n || src[j+9] == ' ' || src[j+9] == '\t') {
				lineEnd := strings.IndexByte(src[j:], '\n')
				var line string
				if lineEnd < 0 {
					line = src[j:]
					i = n
				} else {
					line = src[j : j+lineEnd]
					i = j + lineEnd + 1
				}
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					delimiter = fields[1]
				}
				atLineStart = true
				continue
			}
		}

		// Detect comments outside quotes
		if !inSingle && !inDouble && !inBacktick {
			if ch == '#' {
				inLineComment = true
				i++
				continue
			}
			if ch == '-' && i+1 < n && src[i+1] == '-' && (i+2 == n || src[i+2] == ' ' || src[i+2] == '\t' || src[i+2] == '\r' || src[i+2] == '\n') {
				inLineComment = true
				i += 2
				continue
			}
			if ch == '/' && i+1 < n && src[i+1] == '*' {
				// Preserve MySQL conditional comments /*!40101 ... */ except MariaDB sandbox directive /*M!999999\- ... */
				if i+2 < n && src[i+2] == '!' {
					cur.WriteByte(ch)
					i++
					atLineStart = false
					continue
				}
				inBlockComment = true
				i += 2
				continue
			}
		}

		// Quote handling
		if ch == '\\' && (inSingle || inDouble) && i+1 < n {
			cur.WriteByte(ch)
			cur.WriteByte(src[i+1])
			i += 2
			atLineStart = false
			continue
		}
		if ch == '\'' && !inDouble && !inBacktick {
			inSingle = !inSingle
			cur.WriteByte(ch)
			i++
			atLineStart = false
			continue
		}
		if ch == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
			cur.WriteByte(ch)
			i++
			atLineStart = false
			continue
		}
		if ch == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
			cur.WriteByte(ch)
			i++
			atLineStart = false
			continue
		}

		// Delimiter match outside quotes
		if !inSingle && !inDouble && !inBacktick && len(delimiter) > 0 && i+len(delimiter) <= n && src[i:i+len(delimiter)] == delimiter {
			s := strings.TrimSpace(cur.String())
			if s != "" {
				stmts = append(stmts, s)
			}
			cur.Reset()
			i += len(delimiter)
			atLineStart = false
			continue
		}

		cur.WriteByte(ch)
		atLineStart = (ch == '\n')
		i++
	}

	if tail := strings.TrimSpace(cur.String()); tail != "" {
		stmts = append(stmts, tail)
	}

	return stmts, nil
}

// execSQLFileFallback executes SQL statements from a file directly over a *sql.DB connection
// when native CLI clients (mysql / psql) are not installed on the host.
func execSQLFileFallback(db *sql.DB, sqlFilePath string) error {
	rc, err := openMaybeGzip(sqlFilePath)
	if err != nil {
		return err
	}
	defer rc.Close()

	stmts, err := splitSQLStatements(rc)
	if err != nil {
		return fmt.Errorf("failed to parse SQL file: %w", err)
	}

	for idx, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			preview := stmt
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			return fmt.Errorf("SQL error at statement %d (%s): %w", idx+1, preview, err)
		}
	}
	return nil
}

// parseSnapshotDBName extracts the database name from a snapshot filename.
// Handles both table snapshots (db__table_YYYYMMDD_HHMMSS.sql) and full DB snapshots (db_YYYYMMDD_HHMMSS.sql).
func parseSnapshotDBName(filename string) (string, error) {
	name := strings.TrimSuffix(filename, ".sql")
	if strings.Contains(name, "__") {
		parts := strings.SplitN(name, "__", 2)
		if parts[0] != "" {
			return parts[0], nil
		}
	}
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid snapshot filename: %s", filename)
	}
	return strings.Join(parts[:len(parts)-2], "_"), nil
}

