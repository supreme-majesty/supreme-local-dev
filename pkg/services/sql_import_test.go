package services

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenMaybeGzip_PlainAndGzip(t *testing.T) {
	tmpDir := t.TempDir()
	sqlText := "CREATE TABLE t (id INT); INSERT INTO t VALUES (1);\n"

	// Plain .sql
	plainPath := filepath.Join(tmpDir, "dump.sql")
	if err := os.WriteFile(plainPath, []byte(sqlText), 0644); err != nil {
		t.Fatal(err)
	}
	rc, err := openMaybeGzip(plainPath)
	if err != nil {
		t.Fatalf("openMaybeGzip plain failed: %v", err)
	}
	gotPlain, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(gotPlain) != sqlText {
		t.Errorf("plain content mismatch: got %q", string(gotPlain))
	}

	// Gzipped .sql.gz
	gzPath := filepath.Join(tmpDir, "dump.sql.gz")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(sqlText))
	_ = gw.Close()
	if err := os.WriteFile(gzPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	rc2, err := openMaybeGzip(gzPath)
	if err != nil {
		t.Fatalf("openMaybeGzip gz failed: %v", err)
	}
	gotGz, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if string(gotGz) != sqlText {
		t.Errorf("gzip content mismatch: got %q", string(gotGz))
	}
}

func TestSplitSQLStatements(t *testing.T) {
	dump := `/*M!999999\- enable the sandbox mode */
-- Header comment
# Hash comment
CREATE TABLE users (
  id INT PRIMARY KEY,
  note VARCHAR(255) DEFAULT 'a;b;c'
);
/* multi-line
   comment with ; inside */
INSERT INTO users VALUES (1, 'it\'s; fine');
DELIMITER $$
CREATE PROCEDURE ping()
BEGIN
  SELECT 1;
END$$
DELIMITER ;
SELECT COUNT(*) FROM users;
`
	stmts, err := splitSQLStatements(strings.NewReader(dump))
	if err != nil {
		t.Fatalf("splitSQLStatements failed: %v", err)
	}
	if len(stmts) != 4 {
		t.Fatalf("expected 4 statements, got %d: %#v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "CREATE TABLE users") {
		t.Errorf("stmt[0] = %q", stmts[0])
	}
	if !strings.Contains(stmts[1], `it\'s; fine`) {
		t.Errorf("stmt[1] lost escaped quote/semicolon: %q", stmts[1])
	}
	if !strings.HasPrefix(stmts[2], "CREATE PROCEDURE ping()") {
		t.Errorf("stmt[2] delimiter handling failed: %q", stmts[2])
	}
	if stmts[3] != "SELECT COUNT(*) FROM users" {
		t.Errorf("stmt[3] = %q", stmts[3])
	}
}

func TestBuildMySQLCLIArgs(t *testing.T) {
	args, env := buildMySQLCLIArgs(ConnectionConfig{
		Host:     "host.docker.internal",
		Port:     "33066",
		User:     "root",
		Password: "secret",
	}, "tailormeasure")

	joined := strings.Join(args, " ")
	for _, want := range []string{"--protocol=TCP", "-h host.docker.internal", "-P 33066", "-u root", "tailormeasure"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "secret") {
		t.Errorf("password must not appear in CLI args: %q", joined)
	}
	foundPwd := false
	for _, e := range env {
		if e == "MYSQL_PWD=secret" {
			foundPwd = true
			break
		}
	}
	if !foundPwd {
		t.Errorf("expected MYSQL_PWD=secret in env")
	}
}

func TestParseSnapshotDBName(t *testing.T) {
	db, err := parseSnapshotDBName("my_app_db_20261119_103000.sql")
	if err != nil || db != "my_app_db" {
		t.Errorf("full snapshot = (%q, %v), want my_app_db", db, err)
	}
	db2, err := parseSnapshotDBName("my_app_db__users_20261119_103000.sql")
	if err != nil || db2 != "my_app_db" {
		t.Errorf("table snapshot = (%q, %v), want my_app_db", db2, err)
	}
}

