package services

import (
	"testing"

	"github.com/docker/docker/api/types"
)

func TestDetectDatabaseDriver(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		image    string
		cname    string
		env      map[string]string
		expected string
	}{
		{
			name:     "SLD V2 Postgres container labeled service=db",
			service:  "db",
			image:    "postgres:16-alpine",
			cname:    "sld-api-12345678-db",
			env:      map[string]string{"POSTGRES_USER": "api"},
			expected: "postgres",
		},
		{
			name:     "SLD V2 Postgres custom image with env fallback",
			service:  "db",
			image:    "custom/db:latest",
			cname:    "sld-api-12345678-db",
			env:      map[string]string{"POSTGRES_DB": "app"},
			expected: "postgres",
		},
		{
			name:     "SLD V2 MySQL container labeled service=db",
			service:  "db",
			image:    "mysql:8.0",
			cname:    "sld-api-12345678-db",
			env:      map[string]string{"MYSQL_DATABASE": "api"},
			expected: "mysql",
		},
		{
			name:     "PostGIS container",
			image:    "postgis/postgis:16-3.4",
			cname:    "geo-db-1",
			expected: "postgres",
		},
		{
			name:     "MariaDB container",
			image:    "mariadb:11.4",
			cname:    "shop-mariadb-1",
			expected: "mysql",
		},
		{
			name:     "Non-database container",
			image:    "redis:7-alpine",
			cname:    "app-redis-1",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectDatabaseDriver(tc.service, tc.image, tc.cname, tc.env)
			if got != tc.expected {
				t.Errorf("detectDatabaseDriver(%q, %q, %q) = %q, want %q", tc.service, tc.image, tc.cname, got, tc.expected)
			}
		})
	}
}

func TestResolveDatabaseCredentials(t *testing.T) {
	t.Run("MySQL prefers root with root password over MYSQL_USER", func(t *testing.T) {
		user, pass, db := resolveDatabaseCredentials("mysql", map[string]string{
			"MYSQL_ROOT_PASSWORD": "rootsecret",
			"MYSQL_USER":          "sail",
			"MYSQL_PASSWORD":      "sailpass",
			"MYSQL_DATABASE":      "laravel",
		})
		if user != "root" || pass != "rootsecret" || db != "laravel" {
			t.Errorf("got (%q, %q, %q), want (root, rootsecret, laravel)", user, pass, db)
		}
	})

	t.Run("MySQL falls back to MYSQL_USER and MYSQL_PASSWORD when root password absent", func(t *testing.T) {
		user, pass, db := resolveDatabaseCredentials("mysql", map[string]string{
			"MYSQL_USER":     "appuser",
			"MYSQL_PASSWORD": "apppassword",
			"MYSQL_DATABASE": "appdb",
		})
		if user != "appuser" || pass != "apppassword" || db != "appdb" {
			t.Errorf("got (%q, %q, %q), want (appuser, apppassword, appdb)", user, pass, db)
		}
	})

	t.Run("MariaDB env vars supported", func(t *testing.T) {
		user, pass, db := resolveDatabaseCredentials("mysql", map[string]string{
			"MARIADB_ROOT_PASSWORD": "mdbsecret",
			"MARIADB_DATABASE":      "shop",
		})
		if user != "root" || pass != "mdbsecret" || db != "shop" {
			t.Errorf("got (%q, %q, %q), want (root, mdbsecret, shop)", user, pass, db)
		}
	})

	t.Run("MySQL allow empty password", func(t *testing.T) {
		user, pass, _ := resolveDatabaseCredentials("mysql", map[string]string{
			"MYSQL_ALLOW_EMPTY_PASSWORD": "yes",
		})
		if user != "root" || pass != "" {
			t.Errorf("got (%q, %q), want (root, '')", user, pass)
		}
	})

	t.Run("Postgres defaults and overrides", func(t *testing.T) {
		user, pass, db := resolveDatabaseCredentials("postgres", map[string]string{
			"POSTGRES_USER":     "sld",
			"POSTGRES_PASSWORD": "pgsecret",
			"POSTGRES_DB":       "analytics",
		})
		if user != "sld" || pass != "pgsecret" || db != "analytics" {
			t.Errorf("got (%q, %q, %q), want (sld, pgsecret, analytics)", user, pass, db)
		}

		user, pass, db = resolveDatabaseCredentials("postgres", map[string]string{})
		if user != "postgres" || pass != "" || db != "postgres" {
			t.Errorf("got (%q, %q, %q), want (postgres, '', postgres)", user, pass, db)
		}
	})
}

func TestResolveDatabaseEndpoint(t *testing.T) {
	mysqlPorts := []types.Port{{PrivatePort: 3306, PublicPort: 3307}}
	pgPorts := []types.Port{{PrivatePort: 5432, PublicPort: 5439}}

	t.Run("Inside Docker sharing a network uses container name", func(t *testing.T) {
		host, port := resolveDatabaseEndpoint("mysql", "app-db-1", true, true, mysqlPorts, "172.20.0.3")
		if host != "app-db-1" || port != "3306" {
			t.Errorf("got %s:%s, want app-db-1:3306", host, port)
		}
	})

	t.Run("Inside Docker on different network with published port uses host.docker.internal", func(t *testing.T) {
		host, port := resolveDatabaseEndpoint("mysql", "other-db-1", true, false, mysqlPorts, "172.21.0.4")
		if host != "host.docker.internal" || port != "3307" {
			t.Errorf("got %s:%s, want host.docker.internal:3307", host, port)
		}
	})

	t.Run("Inside Docker without shared network or published port uses container IP", func(t *testing.T) {
		host, port := resolveDatabaseEndpoint("postgres", "isolated-pg-1", true, false, nil, "172.25.0.8")
		if host != "172.25.0.8" || port != "5432" {
			t.Errorf("got %s:%s, want 172.25.0.8:5432", host, port)
		}
	})

	t.Run("On host uses 127.0.0.1 and published port", func(t *testing.T) {
		host, port := resolveDatabaseEndpoint("postgres", "pg-1", false, false, pgPorts, "172.25.0.8")
		if host != "127.0.0.1" || port != "5439" {
			t.Errorf("got %s:%s, want 127.0.0.1:5439", host, port)
		}
	})
}

func TestReplacePostgresDB(t *testing.T) {
	got := replacePostgresDB("postgres://sld:secret@postgres:5432/app?sslmode=disable", "analytics")
	want := "postgres://sld:secret@postgres:5432/analytics?sslmode=disable"
	if got != want {
		t.Errorf("replacePostgresDB = %q, want %q", got, want)
	}

	gotNoQuery := replacePostgresDB("postgres://sld:secret@127.0.0.1:5432/postgres", "mydb")
	wantNoQuery := "postgres://sld:secret@127.0.0.1:5432/mydb"
	if gotNoQuery != wantNoQuery {
		t.Errorf("replacePostgresDB (no query) = %q, want %q", gotNoQuery, wantNoQuery)
	}
}

func TestMightBeDatabaseContainer_SkipsBackupHelpers(t *testing.T) {
	if mightBeDatabaseContainer("", "mysql:8.4", "credora-backup-1") {
		t.Error("expected -backup container to be skipped")
	}
	if mightBeDatabaseContainer("", "mysql:8.0", "sunport_backup") {
		t.Error("expected _backup container to be skipped")
	}
	if !mightBeDatabaseContainer("", "mysql:8.4", "credora-db-1") {
		t.Error("expected real db container to be detected")
	}
}
