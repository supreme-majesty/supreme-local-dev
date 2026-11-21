package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ContainerExecFunc runs cmd inside a running container with env added,
// streaming stdin/stdout/stderr, and returns the command's exit code.
type ContainerExecFunc func(ctx context.Context, container string, cmd, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)

// dbToolTimeout bounds one dump, restore or clone.
const dbToolTimeout = 2 * time.Hour

type dbToolKind int

const (
	toolDump   dbToolKind = iota // mysqldump / mariadb-dump / pg_dump
	toolClient                   // mysql / mariadb / psql
)

// dbTool is one invocation of a database client tool, run either inside the
// database server's own container or with the clients bundled in SLD's image.
//
// Inside the container the tools are always the server's own version: the
// bundled clients are MariaDB's and one PostgreSQL major version, and pg_dump
// refuses to dump a newer server while MariaDB's dump tool has known
// incompatibilities with MySQL 8.
type dbTool struct {
	svc       *DatabaseService
	kind      dbToolKind
	driver    string
	cfg       ConnectionConfig
	container string
	extra     []string
}

func (d *DatabaseService) tool(kind dbToolKind, extra ...string) *dbTool {
	d.mu.RLock()
	cfg := d.activeConfig
	driver := d.driverName
	ce := d.ContainerExec
	d.mu.RUnlock()
	if driver == "" {
		driver = "mysql"
	}
	t := &dbTool{svc: d, kind: kind, driver: driver, cfg: cfg, extra: extra}
	if ce != nil {
		t.container = cfg.Container
	}
	return t
}

// InContainer reports whether the tool runs inside the server's container.
func (t *dbTool) InContainer() bool { return t.container != "" }

func (t *dbTool) name() string {
	switch {
	case t.driver == "postgres" && t.kind == toolDump:
		return "pg_dump"
	case t.driver == "postgres":
		return "psql"
	case t.kind == toolDump:
		return "mysqldump"
	default:
		return "mysql"
	}
}

// Run executes the tool, reading stdin (may be nil) and writing stdout.
func (t *dbTool) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	var stderr strings.Builder
	var err error
	if t.InContainer() {
		err = t.runInContainer(ctx, stdin, stdout, &stderr)
	} else {
		err = t.runLocal(ctx, stdin, stdout, &stderr)
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s failed: %s", t.name(), msg)
		}
		return fmt.Errorf("%s failed: %w", t.name(), err)
	}
	return nil
}

func (t *dbTool) runInContainer(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	user := t.cfg.User
	var cmd, env []string
	if t.driver == "postgres" {
		if user == "" {
			user = "postgres"
		}
		// The server listens on its default port inside its own container,
		// whatever port it is published on.
		cmd = append([]string{t.name(), "-h", "127.0.0.1", "-p", "5432", "-U", user}, t.extra...)
		if t.cfg.Password != "" {
			env = append(env, "PGPASSWORD="+t.cfg.Password)
		}
	} else {
		if user == "" {
			user = "root"
		}
		// MariaDB 11 images ship only the mariadb-* names, MySQL images only
		// the mysql* ones; use whichever exists.
		candidates := "mariadb-dump mysqldump"
		if t.kind == toolClient {
			candidates = "mariadb mysql"
		}
		script := `for b in ` + candidates + `; do command -v "$b" >/dev/null 2>&1 && exec "$b" "$@"; done; echo "no ` + t.name() + ` in this container" >&2; exit 127`
		cmd = append([]string{"sh", "-c", script, "sh", "--protocol=TCP", "-h", "127.0.0.1", "-P", "3306", "-u", user}, t.extra...)
		if t.cfg.Password != "" {
			env = append(env, "MYSQL_PWD="+t.cfg.Password)
		}
	}
	code, err := t.svc.ContainerExec(ctx, t.container, cmd, env, stdin, stdout, stderr)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit code %d", code)
	}
	return nil
}

func (t *dbTool) runLocal(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	var args, env []string
	if t.driver == "postgres" {
		args, env = buildPostgresCLIArgs(t.cfg, t.extra...)
	} else {
		args, env = buildMySQLCLIArgs(t.cfg, t.extra...)
	}
	bin := t.name()
	if _, err := exec.LookPath(bin); err != nil {
		alt := map[string]string{"mysqldump": "mariadb-dump", "mysql": "mariadb"}[bin]
		if alt == "" {
			return fmt.Errorf("%s not found in PATH", bin)
		}
		if _, err2 := exec.LookPath(alt); err2 != nil {
			return fmt.Errorf("%s not found in PATH", bin)
		}
		bin = alt
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// dumpToFile runs a dump tool into path, removing the file if it fails.
func (t *dbTool) dumpToFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbToolTimeout)
	defer cancel()
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}
	runErr := t.Run(ctx, nil, f)
	closeErr := f.Close()
	if runErr == nil {
		runErr = closeErr
	}
	if runErr != nil {
		_ = os.Remove(path)
	}
	return runErr
}

// restoreFromFile feeds a (possibly gzipped) SQL file to a client tool.
func (t *dbTool) restoreFromFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbToolTimeout)
	defer cancel()
	r, err := openMaybeGzip(path)
	if err != nil {
		return fmt.Errorf("failed to open SQL file: %w", err)
	}
	defer r.Close()
	return t.Run(ctx, r, io.Discard)
}

// snapshotDumpArgs returns the dump arguments for one database (and table).
func (d *DatabaseService) snapshotDumpArgs(database, table string) []string {
	if d.DriverName() == "postgres" {
		args := []string{"--clean", "--if-exists", "-d", database} // restorable over the same database
		if table != "" {
			args = append(args, "-t", table)
		}
		return args
	}
	args := []string{"--single-transaction", "--quick", database}
	if table != "" {
		args = append(args, table)
	}
	return args
}

func (d *DatabaseService) restoreClientArgs(database string) []string {
	if d.DriverName() == "postgres" {
		return []string{"-v", "ON_ERROR_STOP=1", "-d", database}
	}
	return []string{"--default-character-set=utf8mb4", database}
}

// pipeTools runs dump | client, e.g. to clone a database.
func pipeTools(dump, client *dbTool) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbToolTimeout)
	defer cancel()
	pr, pw := io.Pipe()
	dumpErr := make(chan error, 1)
	go func() {
		err := dump.Run(ctx, nil, pw)
		pw.CloseWithError(err)
		dumpErr <- err
	}()
	clientErr := client.Run(ctx, pr, io.Discard)
	_ = pr.Close()
	if err := <-dumpErr; err != nil {
		return err
	}
	return clientErr
}
