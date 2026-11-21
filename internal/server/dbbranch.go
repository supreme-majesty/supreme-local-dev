package server

// Database checkpoints and branches for a project's own database container
// (an SLD `services.database` container or the database service of a Compose
// stack). Everything runs inside that container with its own client tools
// through `docker exec`, so it works whatever port or network the server is
// on, and dumps always match the server version.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// DBServer is a database server container of a project.
type DBServer struct {
	Container string   `json:"container"`
	Engine    string   `json:"engine"` // mysql | mariadb | postgres
	Image     string   `json:"image"`
	State     string   `json:"state"`
	Main      string   `json:"main,omitempty"` // the project's database on this server
	Databases []DBInfo `json:"databases"`
	Error     string   `json:"error,omitempty"`
	cred      dbCred   // from the container's environment
	actual    *reconcile.ActualContainer
}

// DBInfo is one database on a server.
type DBInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	IsMain bool   `json:"is_main,omitempty"`
	Branch string `json:"branch,omitempty"` // branch name when it is <main>__<branch>
	InUse  bool   `json:"in_use,omitempty"` // the project's .env points at it
}

// Checkpoint is a dump of one database kept in SLD's data volume.
type Checkpoint struct {
	ID        string    `json:"id"`
	Container string    `json:"container"`
	Engine    string    `json:"engine"`
	Database  string    `json:"database"`
	Label     string    `json:"label,omitempty"`
	Auto      bool      `json:"auto,omitempty"` // taken automatically before a restore
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"` // compressed bytes
}

type dbCred struct {
	user, password string
}

const (
	branchSep          = "__"
	maxCheckpoints     = 30
	dbOperationTimeout = 2 * time.Hour
)

var (
	branchNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,29}$`)
	checkpointRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)
	systemDBs    = map[string]bool{
		"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
		"postgres": true, "template0": true, "template1": true,
	}
)

// dbEngine classifies a database server image by its repository name
// ("mysql:8.4", "bitnami/mariadb", "postgis/postgis:16"). Backup or exporter
// images that merely mention a database ("databack/mysql-backup") are not
// servers.
func dbEngine(image string) string {
	name := strings.ToLower(image)
	if i := strings.LastIndex(name, "@"); i >= 0 {
		name = name[:i]
	}
	name = path.Base(name)
	if i := strings.Index(name, ":"); i >= 0 {
		name = name[:i]
	}
	switch name {
	case "mysql", "percona", "percona-server", "mysql-server":
		return "mysql"
	case "mariadb":
		return "mariadb"
	case "postgres", "postgis", "timescaledb", "timescaledb-ha", "pgvector":
		return "postgres"
	}
	return ""
}

func (s *Server) registerDBBranches(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/projects/{id}/db", s.handleDBOverview)
	mux.HandleFunc("POST /api/v2/projects/{id}/db/checkpoint", s.handleDBCheckpoint)
	mux.HandleFunc("POST /api/v2/projects/{id}/db/restore", s.handleDBRestoreCheckpoint)
	mux.HandleFunc("POST /api/v2/projects/{id}/db/branch", s.handleDBBranch)
	mux.HandleFunc("POST /api/v2/projects/{id}/db/drop", s.handleDBDropBranch)
	mux.HandleFunc("POST /api/v2/projects/{id}/db/use", s.handleDBUse)
	mux.HandleFunc("DELETE /api/v2/projects/{id}/db/checkpoints/{cp}", s.handleDBDeleteCheckpoint)
}

// --- discovery ---

// projectDBContext is what the database endpoints need about a project.
type projectDBContext struct {
	id      string
	root    string
	servers []*DBServer
	envFile string // project-relative .env that sets the database, if any
	envDB   string // its DB_DATABASE (or the database in DATABASE_URL)
}

func (s *Server) projectDB(ctx context.Context, w http.ResponseWriter, r *http.Request, withDatabases bool) (*projectDBContext, bool) {
	id := r.PathValue("id")
	p, ok := s.lookup(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	pc := &projectDBContext{id: id, root: p.Root}
	pc.envFile, pc.envDB = projectEnvDatabase(p.Root)

	identity, err := s.opts.Env.ProjectIdentity(p.Root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	containers, err := s.opts.Engine.ListProjectContainers(ctx, identity.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	for _, c := range containers {
		// The list shows an image ID instead of its name once the tag has
		// moved to a newer image (after a pull); the container's config keeps
		// the name it was created from.
		img := c.Image
		engine := dbEngine(img)
		var cfgEnv []string
		if info, err := s.opts.Engine.InspectContainer(ctx, c.ID); err == nil && info.Config != nil {
			if engine == "" && info.Config.Image != "" {
				img = info.Config.Image
				engine = dbEngine(img)
			}
			cfgEnv = info.Config.Env
		}
		if engine == "" {
			continue
		}
		srv := &DBServer{Container: c.Name, Engine: engine, Image: img, State: c.State, Databases: []DBInfo{}, actual: c}
		srv.cred = credentialsFromEnv(engine, cfgEnv)
		srv.Main = envValue(cfgEnv, "MARIADB_DATABASE", "MYSQL_DATABASE", "POSTGRES_DB")
		if withDatabases && c.State == "running" {
			if err := s.loadDatabases(ctx, srv, pc.envDB); err != nil {
				srv.Error = err.Error()
			}
		}
		pc.servers = append(pc.servers, srv)
	}
	sort.Slice(pc.servers, func(i, j int) bool { return pc.servers[i].Container < pc.servers[j].Container })
	return pc, true
}

func (pc *projectDBContext) server(name string) (*DBServer, error) {
	for _, srv := range pc.servers {
		if srv.Container == name || (name == "" && len(pc.servers) == 1) {
			if srv.State != "running" {
				return nil, fmt.Errorf("%s is %s; start the project first", srv.Container, srv.State)
			}
			return srv, nil
		}
	}
	if name == "" {
		return nil, errors.New("choose the database container")
	}
	return nil, fmt.Errorf("no database container %s in this project", name)
}

func envValue(env []string, keys ...string) string {
	for _, k := range keys {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, k+"="); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

// credentialsFromEnv picks the most privileged account the official images
// set up from their environment (branches need CREATE DATABASE).
func credentialsFromEnv(engine string, env []string) dbCred {
	if engine == "postgres" {
		user := envValue(env, "POSTGRES_USER")
		if user == "" {
			user = "postgres"
		}
		return dbCred{user: user, password: envValue(env, "POSTGRES_PASSWORD")}
	}
	if pw := envValue(env, "MARIADB_ROOT_PASSWORD", "MYSQL_ROOT_PASSWORD"); pw != "" {
		return dbCred{user: "root", password: pw}
	}
	if envValue(env, "MARIADB_ALLOW_EMPTY_ROOT_PASSWORD", "MYSQL_ALLOW_EMPTY_PASSWORD") != "" {
		return dbCred{user: "root"}
	}
	if user := envValue(env, "MARIADB_USER", "MYSQL_USER"); user != "" {
		return dbCred{user: user, password: envValue(env, "MARIADB_PASSWORD", "MYSQL_PASSWORD")}
	}
	return dbCred{user: "root"}
}

// projectEnvDatabase finds the .env (project root first, then one level of
// sub-folders such as backend/) that names the app's database.
func projectEnvDatabase(root string) (file, database string) {
	candidates := []string{".env"}
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "node_modules" && e.Name() != "vendor" {
				candidates = append(candidates, e.Name()+"/.env")
			}
		}
	}
	for _, rel := range candidates {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, v := range parseEnv(data) {
			switch v.Key {
			case "DB_DATABASE":
				if v.Value != "" {
					return rel, v.Value
				}
			case "DATABASE_URL":
				if u, err := url.Parse(v.Value); err == nil && u.Path != "" && u.Path != "/" {
					if db := strings.TrimPrefix(u.Path, "/"); db != "" {
						database = db
						file = rel
					}
				}
			}
		}
		if database != "" {
			return file, database
		}
	}
	return "", ""
}

// loadDatabases lists the server's databases with their sizes and marks the
// project's main database and its branches.
func (s *Server) loadDatabases(ctx context.Context, srv *DBServer, envDB string) error {
	var q string
	if srv.Engine == "postgres" {
		q = "SELECT datname || chr(9) || pg_database_size(datname) FROM pg_database WHERE NOT datistemplate ORDER BY 1"
	} else {
		q = "SELECT s.SCHEMA_NAME, COALESCE(SUM(t.DATA_LENGTH + t.INDEX_LENGTH), 0) FROM information_schema.SCHEMATA s " +
			"LEFT JOIN information_schema.TABLES t ON t.TABLE_SCHEMA = s.SCHEMA_NAME GROUP BY s.SCHEMA_NAME ORDER BY 1"
	}
	out, err := s.dbQuery(ctx, srv, q)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, size, _ := strings.Cut(line, "\t")
		if name == "" || systemDBs[name] {
			continue
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
		srv.Databases = append(srv.Databases, DBInfo{Name: name, Size: n})
		have[name] = true
	}
	// The project's database: what its .env uses (unless that is a branch),
	// else what the container creates, else the only one there is.
	main := ""
	if envDB != "" && have[envDB] {
		main = envDB
		if base, _, ok := strings.Cut(envDB, branchSep); ok && have[base] {
			main = base
		}
	}
	if main == "" && srv.Main != "" && have[srv.Main] {
		main = srv.Main
	}
	if main == "" && len(srv.Databases) == 1 {
		main = srv.Databases[0].Name
	}
	srv.Main = main
	for i := range srv.Databases {
		d := &srv.Databases[i]
		d.IsMain = d.Name == main
		if main != "" {
			if b, ok := strings.CutPrefix(d.Name, main+branchSep); ok {
				d.Branch = b
			}
		}
		d.InUse = d.Name == envDB
	}
	return nil
}

// --- running client tools in the container ---

// toolCmd returns the command for a client ("client") or dump ("dump") tool
// and its environment.
func toolCmd(srv *DBServer, tool string, args ...string) (cmd, env []string) {
	if srv.Engine == "postgres" {
		bin := "psql"
		if tool == "dump" {
			bin = "pg_dump"
		}
		// The local socket: no host or port to guess.
		cmd = append([]string{bin, "-U", srv.cred.user}, args...)
		if srv.cred.password != "" {
			env = append(env, "PGPASSWORD="+srv.cred.password)
		}
		return cmd, env
	}
	candidates := "mariadb mysql"
	if tool == "dump" {
		candidates = "mariadb-dump mysqldump"
	}
	script := `for b in ` + candidates + `; do command -v "$b" >/dev/null 2>&1 && exec "$b" "$@"; done; echo "no database client in this container" >&2; exit 127`
	cmd = append([]string{"sh", "-c", script, "sh", "-u" + srv.cred.user}, args...)
	if srv.cred.password != "" {
		env = append(env, "MYSQL_PWD="+srv.cred.password)
	}
	return cmd, env
}

func (s *Server) dbRun(ctx context.Context, srv *DBServer, tool string, args []string, stdin io.Reader, stdout io.Writer) error {
	executor, ok := s.opts.Engine.(docker.Executor)
	if !ok {
		return errors.New("docker engine cannot run commands in containers")
	}
	cmd, env := toolCmd(srv, tool, args...)
	var stderr bytes.Buffer
	if stdout == nil {
		stdout = io.Discard
	}
	code, err := executor.Exec(ctx, srv.Container, docker.ExecOptions{Cmd: cmd, Env: env, Stdin: stdin, Stdout: stdout, Stderr: &stderr})
	if err != nil {
		return err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = fmt.Sprintf("exit code %d", code)
		}
		return errors.New(msg)
	}
	return nil
}

// dbQuery runs one SQL statement and returns tab-separated rows.
func (s *Server) dbQuery(ctx context.Context, srv *DBServer, sql string) (string, error) {
	var out bytes.Buffer
	var args []string
	if srv.Engine == "postgres" {
		args = []string{"-d", "postgres", "-At", "-v", "ON_ERROR_STOP=1", "-c", sql}
	} else {
		args = []string{"-N", "-B", "-e", sql}
	}
	if err := s.dbRun(ctx, srv, "client", args, nil, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

func quoteIdent(engine, name string) string {
	if engine == "postgres" {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteLiteral(engine, v string) string {
	if engine != "postgres" {
		v = strings.ReplaceAll(v, `\`, `\\`)
	}
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func (s *Server) dbExists(ctx context.Context, srv *DBServer, name string) (bool, error) {
	var q string
	if srv.Engine == "postgres" {
		q = "SELECT 1 FROM pg_database WHERE datname = " + quoteLiteral(srv.Engine, name)
	} else {
		q = "SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = " + quoteLiteral(srv.Engine, name)
	}
	out, err := s.dbQuery(ctx, srv, q)
	return strings.TrimSpace(out) == "1", err
}

// charsetClause returns " CHARACTER SET x COLLATE y" for an existing MySQL
// database (empty for PostgreSQL or when unknown).
func (s *Server) charsetClause(ctx context.Context, srv *DBServer, name string) string {
	if srv.Engine == "postgres" || name == "" {
		return ""
	}
	out, err := s.dbQuery(ctx, srv, "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = "+quoteLiteral(srv.Engine, name))
	if err != nil {
		return ""
	}
	cs, coll, ok := strings.Cut(strings.TrimSpace(out), "	")
	if !ok || cs == "" || coll == "" {
		return ""
	}
	return " CHARACTER SET " + quoteIdent(srv.Engine, cs) + " COLLATE " + quoteIdent(srv.Engine, coll)
}

// createDatabase creates name; charset is a charsetClause (may be empty).
func (s *Server) createDatabase(ctx context.Context, srv *DBServer, name, charset string) error {
	_, err := s.dbQuery(ctx, srv, "CREATE DATABASE "+quoteIdent(srv.Engine, name)+charset)
	return err
}

func (s *Server) dropDatabase(ctx context.Context, srv *DBServer, name string) error {
	stmt := "DROP DATABASE IF EXISTS " + quoteIdent(srv.Engine, name)
	if srv.Engine == "postgres" {
		stmt += " WITH (FORCE)" // close the app's connections (PostgreSQL 13+)
	}
	_, err := s.dbQuery(ctx, srv, stmt)
	return err
}

func dumpArgs(engine, database string) []string {
	if engine == "postgres" {
		return []string{"--no-owner", "--no-acl", "-d", database}
	}
	// No --databases: the dump has no CREATE DATABASE/USE, so it loads into
	// any database (a branch, or the recreated original).
	return []string{"--single-transaction", "--quick", "--routines", "--triggers", "--no-tablespaces", database}
}

func loadArgs(engine, database string) []string {
	if engine == "postgres" {
		return []string{"-q", "-v", "ON_ERROR_STOP=1", "-d", database}
	}
	return []string{"--default-character-set=utf8mb4", database}
}

// copyDatabase streams a dump of from into to (both on srv).
func (s *Server) copyDatabase(ctx context.Context, srv *DBServer, from, to string) error {
	pr, pw := io.Pipe()
	dumpErr := make(chan error, 1)
	go func() {
		err := s.dbRun(ctx, srv, "dump", dumpArgs(srv.Engine, from), nil, pw)
		pw.CloseWithError(err)
		dumpErr <- err
	}()
	loadErr := s.dbRun(ctx, srv, "client", loadArgs(srv.Engine, to), pr, nil)
	_ = pr.Close()
	if err := <-dumpErr; err != nil {
		return fmt.Errorf("dump of %s: %w", from, err)
	}
	if loadErr != nil {
		return fmt.Errorf("loading into %s: %w", to, loadErr)
	}
	return nil
}

// --- checkpoints on disk ---

func (s *Server) checkpointDir(projectID string) string {
	return filepath.Join(s.opts.Env.DataDir(), "db-checkpoints", projectID)
}

func (s *Server) listCheckpoints(projectID string) []Checkpoint {
	dir := s.checkpointDir(projectID)
	entries, _ := os.ReadDir(dir)
	var out []Checkpoint
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !checkpointRe.MatchString(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var cp Checkpoint
		if json.Unmarshal(data, &cp) == nil && cp.ID == id {
			out = append(out, cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Server) checkpointByID(projectID, id string) (Checkpoint, string, error) {
	if !checkpointRe.MatchString(id) {
		return Checkpoint{}, "", errors.New("invalid checkpoint id")
	}
	for _, cp := range s.listCheckpoints(projectID) {
		if cp.ID == id {
			return cp, filepath.Join(s.checkpointDir(projectID), id+".sql.gz"), nil
		}
	}
	return Checkpoint{}, "", errors.New("checkpoint not found")
}

// takeCheckpoint dumps database into a compressed file plus its metadata.
func (s *Server) takeCheckpoint(ctx context.Context, out io.Writer, projectID string, srv *DBServer, database, label string, auto bool) (Checkpoint, error) {
	dir := s.checkpointDir(projectID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Checkpoint{}, err
	}
	suffix := make([]byte, 3)
	_, _ = rand.Read(suffix)
	now := time.Now().UTC()
	cp := Checkpoint{
		ID: now.Format("20060102-150405") + "-" + hex.EncodeToString(suffix), Container: srv.Container,
		Engine: srv.Engine, Database: database, Label: label, Auto: auto, CreatedAt: now,
	}
	file := filepath.Join(dir, cp.ID+".sql.gz")
	fmt.Fprintf(out, "[SLD] Checkpoint of %s on %s…\n", database, srv.Container)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Checkpoint{}, err
	}
	zw := gzip.NewWriter(f)
	runErr := s.dbRun(ctx, srv, "dump", dumpArgs(srv.Engine, database), nil, zw)
	if err := zw.Close(); runErr == nil {
		runErr = err
	}
	if err := f.Close(); runErr == nil {
		runErr = err
	}
	if runErr != nil {
		_ = os.Remove(file)
		return Checkpoint{}, fmt.Errorf("dump of %s: %w", database, runErr)
	}
	if st, err := os.Stat(file); err == nil {
		cp.Size = st.Size()
	}
	meta, _ := json.MarshalIndent(cp, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, cp.ID+".json"), meta, 0o600); err != nil {
		_ = os.Remove(file)
		return Checkpoint{}, err
	}
	fmt.Fprintf(out, "[SLD] Saved checkpoint %s (%s compressed)\n", cp.ID, humanBytes(cp.Size))
	s.pruneCheckpoints(projectID)
	return cp, nil
}

// pruneCheckpoints keeps the newest maxCheckpoints.
func (s *Server) pruneCheckpoints(projectID string) {
	all := s.listCheckpoints(projectID)
	for _, cp := range all[min(len(all), maxCheckpoints):] {
		s.removeCheckpoint(projectID, cp.ID)
	}
}

func (s *Server) removeCheckpoint(projectID, id string) {
	dir := s.checkpointDir(projectID)
	_ = os.Remove(filepath.Join(dir, id+".sql.gz"))
	_ = os.Remove(filepath.Join(dir, id+".json"))
}

// loadCheckpoint feeds a checkpoint file into database.
func (s *Server) loadCheckpoint(ctx context.Context, srv *DBServer, file, database string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	return s.dbRun(ctx, srv, "client", loadArgs(srv.Engine, database), zr, nil)
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// --- handlers ---

func (s *Server) handleDBOverview(w http.ResponseWriter, r *http.Request) {
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	servers := pc.servers
	if servers == nil {
		servers = []*DBServer{}
	}
	checkpoints := s.listCheckpoints(pc.id)
	if checkpoints == nil {
		checkpoints = []Checkpoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"servers": servers, "checkpoints": checkpoints,
		"env_file": pc.envFile, "env_database": pc.envDB,
	})
}

// startDBJob runs a database operation as a job. Database jobs of a project
// run one at a time, but do not block starting or stopping the project.
func (s *Server) startDBJob(w http.ResponseWriter, pc *projectDBContext, kind string, fn func(ctx context.Context, job *Job) error) {
	job, err := s.jobs.Start(kind, pc.id+"#db", func(_ context.Context, job *Job) error {
		ctx, cancel := context.WithTimeout(context.Background(), dbOperationTimeout)
		defer cancel()
		return fn(ctx, job)
	})
	var busy ErrBusy
	if errors.As(err, &busy) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another database operation is running for this project", "job_id": busy.JobID})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job.mu.Lock()
	job.ProjectID = pc.id
	job.mu.Unlock()
	view, _ := job.Snapshot(0)
	writeJSON(w, http.StatusAccepted, view)
}

func (s *Server) handleDBCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Container string `json:"container"`
		Database  string `json:"database"`
		Label     string `json:"label"`
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	srv, err := pc.server(body.Container)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	db := body.Database
	if db == "" {
		db = srv.Main
	}
	if !hasDatabase(srv, db) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("no database %q on %s", db, srv.Container))
		return
	}
	label := strings.TrimSpace(body.Label)
	if len(label) > 100 {
		label = label[:100]
	}
	s.startDBJob(w, pc, "db-checkpoint", func(ctx context.Context, job *Job) error {
		_, err := s.takeCheckpoint(ctx, job, pc.id, srv, db, label, false)
		return err
	})
}

func hasDatabase(srv *DBServer, name string) bool {
	for _, d := range srv.Databases {
		if d.Name == name {
			return true
		}
	}
	return false
}

// handleDBRestoreCheckpoint rolls a database back to a checkpoint. The
// current state is checkpointed first, so a restore can itself be undone.
func (s *Server) handleDBRestoreCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Checkpoint string `json:"checkpoint"`
		Into       string `json:"into"` // default: the database it was taken from
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	cp, file, err := s.checkpointByID(pc.id, body.Checkpoint)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	srv, err := pc.server(cp.Container)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target := body.Into
	if target == "" {
		target = cp.Database
	}
	if systemDBs[target] {
		writeError(w, http.StatusBadRequest, "cannot restore into a system database")
		return
	}
	s.startDBJob(w, pc, "db-restore", func(ctx context.Context, job *Job) error {
		charset := s.charsetClause(ctx, srv, target)
		if hasDatabase(srv, target) {
			if _, err := s.takeCheckpoint(ctx, job, pc.id, srv, target, "before restoring "+cp.ID, true); err != nil {
				return fmt.Errorf("safety checkpoint failed, nothing was changed: %w", err)
			}
			fmt.Fprintf(job, "[SLD] Recreating %s…\n", target)
			if err := s.dropDatabase(ctx, srv, target); err != nil {
				return err
			}
		}
		if err := s.createDatabase(ctx, srv, target, charset); err != nil {
			return err
		}
		fmt.Fprintf(job, "[SLD] Loading checkpoint %s into %s…\n", cp.ID, target)
		if err := s.loadCheckpoint(ctx, srv, file, target); err != nil {
			return err
		}
		fmt.Fprintf(job, "[SLD] %s is back at %s\n", target, cp.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		return nil
	})
}

// handleDBBranch creates <database>__<name> from the database or from a
// checkpoint of it.
func (s *Server) handleDBBranch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Container  string `json:"container"`
		Database   string `json:"database"`
		Name       string `json:"name"`
		Checkpoint string `json:"checkpoint"`
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !branchNameRe.MatchString(body.Name) {
		writeError(w, http.StatusBadRequest, "branch names use lowercase letters, digits and _ (max 30)")
		return
	}
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	var cp Checkpoint
	var file string
	if body.Checkpoint != "" {
		var err error
		if cp, file, err = s.checkpointByID(pc.id, body.Checkpoint); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		body.Container, body.Database = cp.Container, cp.Database
	}
	srv, err := pc.server(body.Container)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	base := body.Database
	if base == "" {
		base = srv.Main
	}
	if base == "" {
		writeError(w, http.StatusBadRequest, "choose the database to branch")
		return
	}
	if b, _, isBranch := strings.Cut(base, branchSep); isBranch {
		base = b // branch of a branch: name it after the main database
	}
	name := base + branchSep + body.Name
	if len(name) > 63 {
		writeError(w, http.StatusBadRequest, "the branch's database name would be longer than 63 characters")
		return
	}
	if hasDatabase(srv, name) {
		writeError(w, http.StatusConflict, "branch "+body.Name+" already exists")
		return
	}
	from := body.Database
	if from == "" {
		from = srv.Main
	}
	if file == "" && !hasDatabase(srv, from) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("no database %q on %s", from, srv.Container))
		return
	}
	s.startDBJob(w, pc, "db-branch", func(ctx context.Context, job *Job) error {
		fmt.Fprintf(job, "[SLD] Creating %s…\n", name)
		if err := s.createDatabase(ctx, srv, name, s.charsetClause(ctx, srv, from)); err != nil {
			return err
		}
		var err error
		if file != "" {
			fmt.Fprintf(job, "[SLD] Loading checkpoint %s…\n", cp.ID)
			err = s.loadCheckpoint(ctx, srv, file, name)
		} else {
			fmt.Fprintf(job, "[SLD] Copying %s…\n", from)
			err = s.copyDatabase(ctx, srv, from, name)
		}
		if err != nil {
			_ = s.dropDatabase(context.Background(), srv, name)
			return err
		}
		fmt.Fprintf(job, "[SLD] Branch %s is ready\n", name)
		return nil
	})
}

// handleDBDropBranch deletes a branch database. Only <main>__<name>
// databases can be deleted here; the project's own database never.
func (s *Server) handleDBDropBranch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Container string `json:"container"`
		Database  string `json:"database"`
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	srv, err := pc.server(body.Container)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	isBranch := false
	for _, d := range srv.Databases {
		if d.Name == body.Database && d.Branch != "" {
			isBranch = true
		}
	}
	if !isBranch {
		writeError(w, http.StatusBadRequest, body.Database+" is not a branch; only branches can be deleted here")
		return
	}
	if body.Database == pc.envDB {
		writeError(w, http.StatusConflict, "the project uses this branch; switch it back to "+srv.Main+" first")
		return
	}
	if err := s.dropDatabase(r.Context(), srv, body.Database); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dropped": body.Database})
}

// handleDBUse points the project's .env at a database (a branch, or back at
// the main one). The previous .env is backed up.
func (s *Server) handleDBUse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Container string `json:"container"`
		Database  string `json:"database"`
	}
	if err := decodeBody(r, 4096, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pc, ok := s.projectDB(r.Context(), w, r, true)
	if !ok {
		return
	}
	if pc.envFile == "" {
		writeError(w, http.StatusBadRequest, "no .env with DB_DATABASE found in this project")
		return
	}
	srv, err := pc.server(body.Container)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !hasDatabase(srv, body.Database) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("no database %q on %s", body.Database, srv.Container))
		return
	}
	p, err := resolveInProject(pc.root, pc.envFile, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	old, err := os.ReadFile(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	vars := parseEnv(old)
	changed := false
	for i, v := range vars {
		switch v.Key {
		case "DB_DATABASE":
			vars[i].Value, changed = body.Database, true
		case "DATABASE_URL":
			if u, err := url.Parse(v.Value); err == nil && u.Path != "" && u.Path != "/" {
				u.Path = "/" + body.Database
				vars[i].Value, changed = u.String(), true
			}
		}
	}
	if !changed {
		writeError(w, http.StatusBadRequest, pc.envFile+" has no DB_DATABASE or DATABASE_URL")
		return
	}
	if err := s.backupEnv(pc.id, filepath.Base(pc.envFile), old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
		return
	}
	if err := s.writeProjectFile(r.Context(), pc.root, p, updateEnv(old, vars)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"env_file": pc.envFile, "database": body.Database})
}

func (s *Server) handleDBDeleteCheckpoint(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.lookup(id); !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if _, _, err := s.checkpointByID(id, r.PathValue("cp")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.removeCheckpoint(id, r.PathValue("cp"))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": r.PathValue("cp")})
}
