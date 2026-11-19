package services

import (
	"strings"
	"testing"
)

func TestIsSystemDatabase(t *testing.T) {
	reserved := []string{"mysql", "`MySQL`", "information_schema", "phpmyadmin", "postgres", "test"}
	for _, name := range reserved {
		if !IsSystemDatabase(name) {
			t.Errorf("expected %q to be a system catalog", name)
		}
	}
	if IsSystemDatabase("app") || IsSystemDatabase("laravel") {
		t.Fatal("project databases must not be treated as system catalogs")
	}
}

func TestFilterUserDatabases(t *testing.T) {
	in := []string{"mysql", "app", "phpmyadmin", "blog", "information_schema"}
	got := FilterUserDatabases(in)
	if len(got) != 2 || got[0] != "app" || got[1] != "blog" {
		t.Fatalf("FilterUserDatabases = %#v", got)
	}
}

func TestGuardCatalogSQL(t *testing.T) {
	blocked := []string{
		"DROP DATABASE mysql",
		"DROP DATABASE IF EXISTS `phpmyadmin`;",
		"CREATE DATABASE test",
		"ALTER DATABASE postgres RENAME TO app",
		"ALTER DATABASE app RENAME TO mysql",
	}
	for _, q := range blocked {
		if err := GuardCatalogSQL(q); err == nil {
			t.Errorf("expected guard to reject %q", q)
		}
	}
	allowed := []string{
		"SELECT 1",
		"DROP TABLE users",
		"CREATE DATABASE app",
		"DROP DATABASE `blog`",
	}
	for _, q := range allowed {
		if err := GuardCatalogSQL(q); err != nil {
			t.Errorf("expected guard to allow %q: %v", q, err)
		}
	}
}

func TestPhpMyAdminHideDbPattern(t *testing.T) {
	p := PhpMyAdminHideDbPattern()
	if p == "" || p[0] != '^' {
		t.Fatalf("unexpected pattern %q", p)
	}
	for _, name := range []string{"mysql", "phpmyadmin", "postgres"} {
		if !strings.Contains(p, name) {
			t.Errorf("pattern %q missing %s", p, name)
		}
	}
}

func TestReplaceMarkedBlock(t *testing.T) {
	block := phpMyAdminHideBlock()
	original := "<?php\n$i = 1;\n"
	updated, changed := replaceMarkedBlock(original, block)
	if !changed || updated == original {
		t.Fatal("expected first upsert to append the hide block")
	}
	again, changed := replaceMarkedBlock(updated, block)
	if changed {
		t.Fatal("second upsert with the same block should be a no-op")
	}
	if again != updated {
		t.Fatal("content drifted on no-op upsert")
	}
}

func TestDSNHelpers(t *testing.T) {
	t.Setenv("SLD_DB_HOST", "test-db")
	t.Setenv("SLD_DB_PORT", "3307")
	t.Setenv("SLD_DB_USER", "admin")
	t.Setenv("SLD_DB_PASS", "secret")
	if got := MySQLDSN(); got != "admin:secret@tcp(test-db:3307)/?timeout=5s&readTimeout=15s&writeTimeout=15s" {
		t.Fatalf("unexpected MySQLDSN: %s", got)
	}

	t.Setenv("SLD_PG_HOST", "test-pg")
	t.Setenv("SLD_PG_PORT", "5433")
	t.Setenv("SLD_PG_USER", "pguser")
	t.Setenv("SLD_PG_PASS", "pgpass")
	t.Setenv("SLD_PG_DB", "mydb")
	if got := PostgresDSN(); got != "postgres://pguser:pgpass@test-pg:5433/mydb?sslmode=disable&connect_timeout=5" {
		t.Fatalf("unexpected PostgresDSN: %s", got)
	}

	t.Setenv("SLD_STATE_DIR", "/custom/state")
	if got := stateDir(); got != "/custom/state" {
		t.Fatalf("unexpected stateDir: %s", got)
	}
}
