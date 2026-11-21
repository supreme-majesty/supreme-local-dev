package services

// Integration tests against real database servers. They run only when the
// server is given through the environment, e.g.
//
//	SLD_TEST_MYSQL=root:pw@127.0.0.1:3306  SLD_TEST_PG=postgres:pw@127.0.0.1:5432  go test ./pkg/services

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func testConfig(t *testing.T, env string) ConnectionConfig {
	v := os.Getenv(env)
	if v == "" {
		t.Skipf("%s not set", env)
	}
	cred, addr, _ := strings.Cut(v, "@")
	user, pw, _ := strings.Cut(cred, ":")
	host, port, _ := strings.Cut(addr, ":")
	return ConnectionConfig{User: user, Password: pw, Host: host, Port: port}
}

func newTestService(t *testing.T, name string, drv DatabaseDriver, cfg ConnectionConfig) *DatabaseService {
	if err := drv.Connect(cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { drv.Close() })
	return &DatabaseService{Driver: drv, driverName: name, sourceDrivers: map[string]DatabaseDriver{}}
}

// exerciseDriver runs the same scenario on either engine: awkward names,
// concurrent queries on two databases, transactions, parameterised import,
// masking by primary key and literal search.
func exerciseDriver(t *testing.T, s *DatabaseService, createTable func(db string) []string) {
	drv := s.Driver
	dbA, dbB := "sld_it_a", "sld it`\"b" // the second needs quoting everywhere
	for _, db := range []string{dbA, dbB} {
		_ = drv.DeleteDatabase(db)
		if err := drv.CreateDatabase(db); err != nil {
			t.Fatalf("create %q: %v", db, err)
		}
		t.Cleanup(func() { _ = drv.DeleteDatabase(db) })
		for _, stmt := range createTable(db) {
			if _, err := drv.ExecuteQuery(db, stmt, ""); err != nil {
				t.Fatalf("setup %q on %q: %v", stmt, db, err)
			}
		}
	}

	// Each database gets distinct rows; concurrent readers must always see
	// their own database (the old "USE db" on a shared pool did not).
	if err := s.ImportData(dbA, "peo`ple", map[string]string{"n": "name", "e": "email"}, []map[string]interface{}{
		{"n": "O'Brien", "e": "a@example.com"}, {"n": "50% off", "e": "b@example.com"},
	}); err != nil {
		t.Fatalf("import A: %v", err)
	}
	if err := s.ImportData(dbB, "peo`ple", map[string]string{"n": "name", "e": "email"}, []map[string]interface{}{
		{"n": "only-in-b", "e": "c@example.com"},
	}); err != nil {
		t.Fatalf("import B: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 100; i++ {
		db, want := dbA, int64(2)
		if i%2 == 1 {
			db, want = dbB, 1
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := drv.ExecuteQuery(db, "SELECT name FROM "+s.quoteIdent("peo`ple"), "")
			if err != nil {
				errs <- err
				return
			}
			if int64(res.RowCount) != want {
				errs <- fmt.Errorf("%s: got %d rows, want %d", db, res.RowCount, want)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	// A transaction runs on the database it was opened for.
	tx, err := s.BeginTransaction(dbB)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	res, err := drv.ExecuteQuery(dbB, "SELECT name FROM "+s.quoteIdent("peo`ple"), tx)
	if err != nil || res.RowCount != 1 {
		t.Errorf("query in tx on B: rows=%v err=%v", res, err)
	}
	if _, err := drv.ExecArgs(dbB, "DELETE FROM "+s.quoteIdent("peo`ple"), nil, tx); err != nil {
		t.Errorf("delete in tx: %v", err)
	}
	if err := s.RollbackTransaction(tx); err != nil {
		t.Errorf("rollback: %v", err)
	}
	if rows, _ := s.Query(dbB, "SELECT name FROM "+s.quoteIdent("peo`ple"), nil); len(rows) != 1 {
		t.Errorf("rollback did not restore the row: %d rows", len(rows))
	}

	// Search treats the text literally: "%" must not match everything.
	results, err := drv.GlobalSearch(dbA, "50%")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if n := countMatches(results); n != 1 {
		t.Errorf("search for '50%%' matched %d rows, want 1", n)
	}
	results, err = drv.GlobalSearch(dbA, "O'Brien")
	if err != nil {
		t.Fatalf("search with quote: %v", err)
	}
	if n := countMatches(results); n != 1 {
		t.Errorf("search for O'Brien matched %d rows, want 1", n)
	}

	// Masking changes every row's email, addressed by the primary key.
	if err := s.AnonymizeTable(MaskingConfig{Database: dbA, Table: "peo`ple", Columns: map[string]string{"email": "EMAIL"}}); err != nil {
		t.Fatalf("mask: %v", err)
	}
	rows, _ := s.Query(dbA, "SELECT name, email FROM "+s.quoteIdent("peo`ple"), nil)
	for _, r := range rows {
		if e := fmt.Sprint(r["email"]); e == "a@example.com" || e == "b@example.com" {
			t.Errorf("row %v was not masked", r)
		}
		if n := fmt.Sprint(r["name"]); n != "O'Brien" && n != "50% off" {
			t.Errorf("masking changed an unmasked column: %v", r)
		}
	}

	cols, err := s.GetTableColumns(dbA, "peo`ple")
	if err != nil || len(cols) != 3 || cols[0].Name != "id" || cols[0].Key != "PRI" {
		t.Errorf("columns: %+v err=%v", cols, err)
	}
}

// exerciseContainerTools snapshots, restores and clones through the database
// container's own tools (SLD_TEST_*_CONTAINER names the container; the test
// needs the Docker socket).
func exerciseContainerTools(t *testing.T, s *DatabaseService, cfg ConnectionConfig, env string) {
	name := os.Getenv(env)
	if name == "" {
		t.Logf("%s not set; skipping container tools", env)
		return
	}
	disc := NewDockerDiscoveryService()
	if !disc.IsAvailable() {
		t.Fatalf("docker not available for %s", env)
	}
	s.ContainerExec = disc.Exec
	cfg.Container = name
	s.activeConfig = cfg
	s.SnapDir = t.TempDir()

	db := "sld_it_tools"
	clone := "sld_it_tools_clone"
	for _, n := range []string{db, clone} {
		_ = s.Driver.DeleteDatabase(n)
		t.Cleanup(func() { _ = s.Driver.DeleteDatabase(n) })
	}
	if err := s.Driver.CreateDatabase(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"CREATE TABLE notes (id INT PRIMARY KEY, body VARCHAR(50))", "INSERT INTO notes VALUES (1, 'kept')"} {
		if _, err := s.Driver.ExecuteQuery(db, q, ""); err != nil {
			t.Fatal(err)
		}
	}

	if !s.tool(toolDump).InContainer() {
		t.Fatal("tool should run in the container")
	}
	snap, err := s.CreateSnapshot(db, "")
	if err != nil {
		t.Fatalf("snapshot in container: %v", err)
	}
	if snap.Size == 0 {
		t.Fatal("empty snapshot")
	}
	if _, err := s.Driver.ExecuteQuery(db, "DELETE FROM notes", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreSnapshot(snap.Filename); err != nil {
		t.Fatalf("restore in container: %v", err)
	}
	if rows, _ := s.Query(db, "SELECT body FROM notes", nil); len(rows) != 1 {
		t.Errorf("after restore: %d rows, want 1", len(rows))
	}

	if err := s.CloneDatabase(db, clone, CloneOptions{Mode: "both", CreateDatabase: true}); err != nil {
		t.Fatalf("clone in container: %v", err)
	}
	if rows, _ := s.Query(clone, "SELECT body FROM notes", nil); len(rows) != 1 {
		t.Errorf("clone: %d rows, want 1", len(rows))
	}
}

func countMatches(results []SearchResult) int {
	n := 0
	for _, r := range results {
		n += len(r.Matches)
	}
	return n
}

func TestMySQLDriverIntegration(t *testing.T) {
	cfg := testConfig(t, "SLD_TEST_MYSQL")
	s := newTestService(t, "mysql", NewMySQLDriver(), cfg)
	exerciseDriver(t, s, func(string) []string {
		return []string{"CREATE TABLE `peo``ple` (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100), email VARCHAR(100), KEY idx_email (email))"}
	})
	idx, err := s.Driver.GetTableIndexes("sld_it_a", "peo`ple")
	if err != nil || len(idx) != 2 || !idx[0].Primary || idx[1].Name != "idx_email" {
		t.Errorf("indexes: %+v err=%v", idx, err)
	}
	exerciseContainerTools(t, s, cfg, "SLD_TEST_MYSQL_CONTAINER")
}

func TestPostgresDriverIntegration(t *testing.T) {
	cfg := testConfig(t, "SLD_TEST_PG")
	s := newTestService(t, "postgres", NewPostgresDriver(), cfg)
	exerciseDriver(t, s, func(string) []string {
		return []string{`CREATE TABLE "peo` + "`" + `ple" (id SERIAL PRIMARY KEY, name VARCHAR(100), email VARCHAR(100))`}
	})
	exerciseContainerTools(t, s, cfg, "SLD_TEST_PG_CONTAINER")
}
