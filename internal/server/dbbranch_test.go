package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
)

func TestDBEngine(t *testing.T) {
	cases := map[string]string{
		"mysql:8.4":                      "mysql",
		"docker.io/library/mariadb:11.4": "mariadb",
		"bitnami/mariadb":                "mariadb",
		"postgres:17-alpine":             "postgres",
		"postgis/postgis:16-3.4":         "postgres",
		"databack/mysql-backup:latest":   "",
		"phpmyadmin:5":                   "",
		"redis:7":                        "",
		"mysql@sha256:abc":               "mysql",
	}
	for img, want := range cases {
		if got := dbEngine(img); got != want {
			t.Errorf("dbEngine(%q) = %q, want %q", img, got, want)
		}
	}
}

func TestCredentialsFromEnv(t *testing.T) {
	if c := credentialsFromEnv("mysql", []string{"MYSQL_USER=app", "MYSQL_PASSWORD=a", "MYSQL_ROOT_PASSWORD=r"}); c.user != "root" || c.password != "r" {
		t.Errorf("root preferred: %+v", c)
	}
	if c := credentialsFromEnv("mariadb", []string{"MARIADB_USER=app", "MARIADB_PASSWORD=a"}); c.user != "app" || c.password != "a" {
		t.Errorf("app user: %+v", c)
	}
	if c := credentialsFromEnv("mysql", []string{"MYSQL_ALLOW_EMPTY_PASSWORD=yes"}); c.user != "root" || c.password != "" {
		t.Errorf("empty root: %+v", c)
	}
	if c := credentialsFromEnv("postgres", []string{"POSTGRES_PASSWORD=p"}); c.user != "postgres" || c.password != "p" {
		t.Errorf("postgres: %+v", c)
	}
}

func TestProjectEnvDatabase(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"backend/.env": "APP_NAME=x\nDB_DATABASE=\"shop\"\n",
		"web/.env":     "VITE_API=http://api\n",
	})
	if f, db := projectEnvDatabase(root); f != "backend/.env" || db != "shop" {
		t.Errorf("got %q %q", f, db)
	}
	writeTree(t, root, map[string]string{".env": "DATABASE_URL=postgres://u:p@db:5432/blog?sslmode=disable\n"})
	if f, db := projectEnvDatabase(root); f != ".env" || db != "blog" {
		t.Errorf("DATABASE_URL: got %q %q", f, db)
	}
}

func TestToolCmdKeepsPasswordOutOfArgs(t *testing.T) {
	srv := &DBServer{Engine: "mysql", cred: dbCred{user: "root", password: "s3cret"}}
	cmd, env := toolCmd(srv, "dump", "shop")
	if strings.Contains(strings.Join(cmd, " "), "s3cret") || env[0] != "MYSQL_PWD=s3cret" {
		t.Errorf("cmd=%v env=%v", cmd, env)
	}
}

// TestDBBranchesIntegration runs the whole flow against real MariaDB and
// PostgreSQL containers labelled as a project's (SLD_TEST_DOCKER=1; needs the
// Docker socket).
func TestDBBranchesIntegration(t *testing.T) {
	if os.Getenv("SLD_TEST_DOCKER") == "" {
		t.Skip("SLD_TEST_DOCKER not set")
	}
	engines := []struct {
		name, image string
		env         []string
		createTable string
	}{
		{"mariadb", "mariadb:11", []string{"MARIADB_ROOT_PASSWORD=pw", "MARIADB_DATABASE=shop"}, "CREATE TABLE shop.notes (id INT PRIMARY KEY, body VARCHAR(50)); INSERT INTO shop.notes VALUES (1,'one'),(2,'two')"},
		{"mysql", "mysql:8.4", []string{"MYSQL_ROOT_PASSWORD=pw", "MYSQL_DATABASE=shop"}, "CREATE TABLE shop.notes (id INT PRIMARY KEY, body VARCHAR(50)); INSERT INTO shop.notes VALUES (1,'one'),(2,'two')"},
		{"postgres", "postgres:17-alpine", []string{"POSTGRES_PASSWORD=pw", "POSTGRES_DB=shop"}, ""},
	}
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) { runDBBranchFlow(t, e.image, e.env, e.createTable) })
	}
}

func runDBBranchFlow(t *testing.T, img string, env []string, mysqlSetup string) {
	ctx := context.Background()
	t.Setenv("SLD_TLD", "")
	ws := t.TempDir()
	writeTree(t, ws, map[string]string{
		"shop/.git/HEAD":        "x",
		"shop/.sld/config.yaml": "version: 2\napps:\n  web: {runtime: static}\n",
		"shop/.env":             "APP_NAME=shop\nDB_DATABASE=shop\n",
	})
	henv := hostenv.Host(t.TempDir())
	identity, err := henv.ProjectIdentity(filepath.Join(ws, "shop"))
	if err != nil {
		t.Fatal(err)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	if rc, err := cli.ImagePull(ctx, img, image.PullOptions{}); err == nil {
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	name := "sld-dbtest-" + identity.ID
	_ = cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	created, err := cli.ContainerCreate(ctx, &container.Config{
		Image: img, Env: env,
		Labels: map[string]string{docker.LabelSLD: "true", docker.LabelProject: identity.ID},
	}, nil, nil, nil, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	})
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}

	engine, err := docker.NewDockerEngine()
	if err != nil {
		t.Fatal(err)
	}
	a, token := newAuth(t)
	mux := http.NewServeMux()
	srv := New(Options{Engine: engine, Env: henv, Auth: a, Workspaces: []string{ws}, Version: "test"})
	srv.Register(mux)
	h := a.Middleware(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Host = "localhost"
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	base := "/api/v2/projects/" + identity.ID + "/db"

	// Wait for the server to accept queries (images run an init server first).
	dbsrv := &DBServer{Container: name, Engine: dbEngine(img), cred: credentialsFromEnv(dbEngine(img), env)}
	pgSetup := []string{"CREATE TABLE notes (id INT PRIMARY KEY, body VARCHAR(50))", "INSERT INTO notes VALUES (1,'one'),(2,'two')"}
	query := func(db, sql string) (string, error) {
		if dbsrv.Engine == "postgres" {
			var out strings.Builder
			err := srv.dbRun(ctx, dbsrv, "client", []string{"-d", db, "-At", "-v", "ON_ERROR_STOP=1", "-c", sql}, nil, &out)
			return out.String(), err
		}
		return srv.dbQuery(ctx, dbsrv, strings.ReplaceAll(sql, "notes", quoteIdent("mysql", db)+".notes"))
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		if out, err := srv.dbQuery(ctx, dbsrv, "SELECT 1"); err == nil && strings.TrimSpace(out) == "1" {
			if dbsrv.Engine == "postgres" {
				// The init server also answers; wait until shop exists on the final one.
				if ok, _ := srv.dbExists(ctx, dbsrv, "shop"); ok {
					time.Sleep(2 * time.Second)
					break
				}
			} else if _, err := srv.dbQuery(ctx, dbsrv, "SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='shop'"); err == nil {
				time.Sleep(3 * time.Second) // MariaDB restarts after init
				if _, err := srv.dbQuery(ctx, dbsrv, "SELECT 1"); err == nil {
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("database did not start")
		}
		time.Sleep(time.Second)
	}
	if mysqlSetup != "" {
		if _, err := srv.dbQuery(ctx, dbsrv, mysqlSetup); err != nil {
			t.Fatal(err)
		}
	} else {
		for _, q := range pgSetup {
			if _, err := query("shop", q); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := func(db string) string {
		out, err := query(db, "SELECT COUNT(*) FROM notes")
		if err != nil {
			return "error: " + err.Error()
		}
		return strings.TrimSpace(out)
	}
	runJob := func(path, body string) JobView {
		t.Helper()
		rec := call("POST", base+path, body)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
		var v JobView
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		j, _ := srv.jobs.Get(v.ID)
		waitDone(t, j)
		v, _ = j.Snapshot(0)
		if v.Status != JobSucceeded {
			t.Fatalf("%s job failed: %s\n%s", path, v.Error, v.Output)
		}
		return v
	}
	type overview struct {
		Servers     []DBServer   `json:"servers"`
		Checkpoints []Checkpoint `json:"checkpoints"`
		EnvDatabase string       `json:"env_database"`
	}
	get := func() overview {
		rec := call("GET", base, "")
		var o overview
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &o) != nil {
			t.Fatalf("overview: %d %s", rec.Code, rec.Body)
		}
		return o
	}

	o := get()
	if len(o.Servers) != 1 || o.Servers[0].Main != "shop" || o.EnvDatabase != "shop" {
		t.Fatalf("overview = %+v", o)
	}

	// Checkpoint, then change the data.
	runJob("/checkpoint", `{"label":"two notes"}`)
	cps := get().Checkpoints
	if len(cps) != 1 || cps[0].Label != "two notes" || cps[0].Size == 0 {
		t.Fatalf("checkpoints = %+v", cps)
	}
	if _, err := query("shop", "DELETE FROM notes WHERE id = 1"); err != nil {
		t.Fatal(err)
	}

	// A branch from the checkpoint has the old data; one from the live
	// database has the new.
	runJob("/branch", `{"name":"before","checkpoint":"`+cps[0].ID+`"}`)
	runJob("/branch", `{"name":"now"}`)
	if c := count("shop__before"); c != "2" {
		t.Errorf("branch from checkpoint has %s rows, want 2", c)
	}
	if c := count("shop__now"); c != "1" {
		t.Errorf("branch from live has %s rows, want 1", c)
	}
	if rec := call("POST", base+"/branch", `{"name":"now"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate branch: %d", rec.Code)
	}

	// Restore rolls back and keeps an automatic checkpoint of what it replaced.
	runJob("/restore", `{"checkpoint":"`+cps[0].ID+`"}`)
	if c := count("shop"); c != "2" {
		t.Errorf("after restore shop has %s rows, want 2", c)
	}
	if cps := get().Checkpoints; len(cps) != 2 || !cps[0].Auto {
		t.Errorf("expected an automatic checkpoint first: %+v", cps)
	}

	// Point the project at a branch, and back.
	if rec := call("POST", base+"/use", `{"database":"shop__now"}`); rec.Code != 200 {
		t.Fatalf("use: %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "shop", ".env"))
	if !strings.Contains(string(data), "DB_DATABASE=shop__now") || !strings.Contains(string(data), "APP_NAME=shop") {
		t.Errorf(".env = %s", data)
	}
	o = get()
	if o.Servers[0].Main != "shop" {
		t.Errorf("main should stay shop while a branch is in use: %+v", o.Servers[0])
	}
	if rec := call("POST", base+"/drop", `{"database":"shop__now"}`); rec.Code != http.StatusConflict {
		t.Errorf("dropping the branch in use: %d", rec.Code)
	}
	if rec := call("POST", base+"/drop", `{"database":"shop"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("dropping main must be refused: %d", rec.Code)
	}
	if rec := call("POST", base+"/use", `{"database":"shop"}`); rec.Code != 200 {
		t.Fatalf("use main: %d %s", rec.Code, rec.Body)
	}
	if rec := call("POST", base+"/drop", `{"database":"shop__now"}`); rec.Code != 200 {
		t.Errorf("drop branch: %d %s", rec.Code, rec.Body)
	}
	if ok, _ := srv.dbExists(ctx, dbsrv, "shop__now"); ok {
		t.Error("branch still exists")
	}

	if rec := call("DELETE", base+"/checkpoints/"+cps[0].ID, ""); rec.Code != 200 {
		t.Errorf("delete checkpoint: %d", rec.Code)
	}
}
