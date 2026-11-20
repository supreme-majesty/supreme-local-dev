package v2

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	goruntime "runtime"
	"strings"

	"github.com/moby/term"
	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/config"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// RegisterExecCommands adds exec, shell, and passthrough commands to the V2 subcommand.
func RegisterExecCommands(v2Cmd *cobra.Command) {
	// --- sld v2 shell ---
	var shellService string
	shellCmd := &cobra.Command{
		Use:   "shell [app|service|path]",
		Short: "Open an interactive shell in a project container (default: the main app)",
		Long: `Opens an interactive shell inside a running project container.
By default connects to the project's main app (for PHP apps, the PHP container).
Name an app from .sld/config.yaml or a service (php, nginx, db, redis, mailpit)
as an argument or with --service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			target := shellService
			if len(args) > 0 {
				if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
					projectRoot = args[0]
				} else {
					target = args[0]
				}
			}
			return execInService(projectRoot, target, nil)
		},
	}
	shellCmd.Flags().StringVarP(&shellService, "service", "s", "", "App name or service (php, nginx, db, redis, mailpit)")
	v2Cmd.AddCommand(shellCmd)

	// --- sld v2 exec ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "exec [--service <app|service>] <command...>",
		Short:              "Execute a command inside a project container (default: the main app)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, cmdArgs := "", args
			switch {
			case len(cmdArgs) >= 3 && (cmdArgs[0] == "--service" || cmdArgs[0] == "-s"):
				target, cmdArgs = cmdArgs[1], cmdArgs[2:]
			case len(cmdArgs) >= 2 && strings.HasPrefix(cmdArgs[0], "--service="):
				target, cmdArgs = strings.TrimPrefix(cmdArgs[0], "--service="), cmdArgs[1:]
			}
			if len(cmdArgs) == 0 {
				return fmt.Errorf("usage: sld v2 exec [--service <app|service>] <command> [args...]")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInService(cwd, target, cmdArgs)
		},
	})

	// Tool passthroughs run in the first container of the matching runtime.
	for _, tool := range []struct {
		use, short, service string
		prefix              []string
	}{
		{"php", "Run PHP CLI in the project's PHP container", docker.ServicePHP, []string{"php"}},
		{"composer", "Run Composer in the project's PHP container", docker.ServicePHP, []string{"composer"}},
		{"artisan", "Run Laravel Artisan in the project's PHP container", docker.ServicePHP, []string{"php", "artisan"}},
		{"python", "Run Python in the project's Python app", config.RuntimePython, []string{"python"}},
		{"pip", "Run pip in the project's Python app", config.RuntimePython, []string{"pip"}},
		{"bundle", "Run Bundler in the project's Ruby app", config.RuntimeRuby, []string{"bundle"}},
		{"rails", "Run Rails in the project's Ruby app", config.RuntimeRuby, []string{"bundle", "exec", "rails"}},
		{"go", "Run the Go toolchain in the project's Go app", config.RuntimeGo, []string{"go"}},
	} {
		tool := tool
		v2Cmd.AddCommand(&cobra.Command{
			Use:                tool.use + " [args...]",
			Short:              tool.short,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				return execInService(cwd, tool.service, append(append([]string{}, tool.prefix...), args...))
			},
		})
	}

	// --- sld v2 npm / npx ---
	for _, tool := range []string{"npm", "npx"} {
		tool := tool
		v2Cmd.AddCommand(&cobra.Command{
			Use:                tool + " [args...]",
			Short:              "Run " + tool + " in the project's Node app (or a temporary Node container)",
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				return execNodeTool(cwd, append([]string{tool}, args...))
			},
		})
	}
}

// langServices are app containers whose main process runs as the developer's
// UID; commands exec'd into them do the same so created files are not root-owned.
var langServices = map[string]bool{
	config.RuntimeNode: true, config.RuntimePython: true, config.RuntimeRuby: true, config.RuntimeGo: true,
}

// resolveTarget picks the running container for an app name or service.
// An empty target means the project's main app.
func resolveTarget(containers []*reconcile.ActualContainer, cfg *config.Config, target string) (*reconcile.ActualContainer, error) {
	return pickContainer(containers, cfg, target, true)
}

// ResolveContainer finds a project container by app name or service (empty =
// the main app). includeStopped also considers stopped containers (for logs).
func ResolveContainer(containers []*reconcile.ActualContainer, cfg *config.Config, target string, includeStopped bool) (*reconcile.ActualContainer, error) {
	return pickContainer(containers, cfg, target, !includeStopped)
}

// LoadProjectConfig reads and validates a project's .sld/config.yaml.
func LoadProjectConfig(projectRoot string) (*config.Config, []byte, error) {
	return loadProjectConfig(projectRoot)
}

// pickContainer is resolveTarget with the option to consider stopped containers (for logs).
func pickContainer(containers []*reconcile.ActualContainer, cfg *config.Config, target string, runningOnly bool) (*reconcile.ActualContainer, error) {
	running := make([]*reconcile.ActualContainer, 0, len(containers))
	for _, c := range containers {
		if !runningOnly || c.State == "running" {
			running = append(running, c)
		}
	}

	if target == "" && cfg != nil {
		if primary := cfg.PrimaryApp(); primary != nil {
			target = primary.Name
		}
	}

	// 1. App name: prefer the app's code container over its nginx sidecar.
	var appMatch *reconcile.ActualContainer
	for _, c := range running {
		if c.Labels[docker.LabelApp] == target && target != "" {
			if c.Service != docker.ServiceNginx {
				return c, nil
			}
			appMatch = c
		}
	}
	if appMatch != nil {
		return appMatch, nil
	}
	// 2. Service / runtime name (php, nginx, db, redis, mailpit, node, ...).
	for _, c := range running {
		if strings.EqualFold(c.Service, target) {
			return c, nil
		}
	}
	// 3. Projects started before apps had labels: fall back to the PHP container.
	if target == "" {
		for _, c := range running {
			if c.Service == docker.ServicePHP {
				return c, nil
			}
		}
	}

	var available []string
	for _, c := range running {
		name := c.Service
		if app := c.Labels[docker.LabelApp]; app != "" {
			name = app + " (" + c.Service + ")"
		}
		available = append(available, name)
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no containers are running for this project. Run 'sld v2 start' first")
	}
	return nil, fmt.Errorf("no running container for %q. Running: %s", target, strings.Join(available, ", "))
}

// execInService executes a command in a project container through the Docker
// API and exits with the command's exit code. A nil command opens a shell.
func execInService(projectRoot, target string, command []string) error {
	runner, err := lazyRunner()
	if err != nil {
		return err
	}
	engine := runner.orch.Engine()
	defer engine.Close()

	identity, err := runner.identity(projectRoot)
	if err != nil {
		return err
	}
	cfg, _, _ := loadProjectConfig(projectRoot) // optional: only used to find the main app

	containers, err := engine.ListProjectContainers(context.Background(), identity.ID)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	c, err := resolveTarget(containers, cfg, target)
	if err != nil {
		return err
	}

	if command == nil {
		command = []string{"sh"}
		if c.Service == docker.ServicePHP {
			command = []string{"bash"}
		}
	}

	executor, ok := engine.(docker.Executor)
	if !ok {
		return fmt.Errorf("docker engine does not support exec")
	}
	user := ""
	if langServices[c.Service] {
		uid, gid := runner.orch.ProjectOwner(projectRoot)
		user = uid + ":" + gid
	}

	fmt.Fprintf(os.Stderr, "[SLD] Executing in %s: %s\n", c.Name, formatCommand(command))
	code, err := runAttached(command, func(ctx context.Context, opts docker.ExecOptions) (int, error) {
		opts.User = user
		return executor.Exec(ctx, c.Name, opts)
	})
	return exitWith(code, err)
}

// execNodeTool runs npm/npx inside a running Node app (where node_modules
// lives in its dependency volume), or in a temporary Node container.
func execNodeTool(projectRoot string, command []string) error {
	runner, err := lazyRunner()
	if err != nil {
		return err
	}
	engine := runner.orch.Engine()

	identity, err := runner.identity(projectRoot)
	if err != nil {
		engine.Close()
		return err
	}
	containers, err := engine.ListProjectContainers(context.Background(), identity.ID)
	if err == nil {
		for _, c := range containers {
			if c.Service == config.RuntimeNode && c.State == "running" {
				engine.Close()
				return execInService(projectRoot, c.Labels[docker.LabelApp], command)
			}
		}
	}
	engine.Close()
	return execInNode(projectRoot, command)
}

// execInNode runs a command in an ephemeral Node container mounted to the project.
func execInNode(projectRoot string, command []string) error {
	runner, err := lazyRunner()
	if err != nil {
		return err
	}
	engine := runner.orch.Engine()
	defer engine.Close()

	identity, err := runner.identity(projectRoot)
	if err != nil {
		return err
	}

	nodeVersion := config.DefaultVersions[config.RuntimeNode]
	if cfg, _, err := loadProjectConfig(projectRoot); err == nil {
		nodeVersion = cfg.NodeVersion()
	}

	appMount, err := runner.env().MountFor(projectRoot, "/app", false)
	if err != nil {
		return err
	}

	desired := &reconcile.DesiredContainer{
		Image:      fmt.Sprintf("node:%s-bookworm-slim", nodeVersion),
		WorkingDir: "/app",
		Mounts:     []reconcile.Mount{appMount},
		// An arbitrary UID may have no home directory in the image.
		Environment: map[string]string{"HOME": "/tmp", "npm_config_cache": "/tmp/.npm"},
	}
	// Preserve the developer's UID/GID on Linux so node_modules and build
	// output are not root-owned (Docker Desktop maps ownership itself).
	if goruntime.GOOS == "linux" {
		uid, gid := runner.orch.ProjectOwner(projectRoot)
		desired.User = uid + ":" + gid
	}
	// Join the project network when the project is running (e.g. to reach its DB).
	if containers, err := engine.ListProjectContainers(context.Background(), identity.ID); err == nil && len(containers) > 0 {
		desired.Networks = []string{identity.NetworkName()}
	}

	executor, ok := engine.(docker.Executor)
	if !ok {
		return fmt.Errorf("docker engine does not support exec")
	}

	fmt.Fprintf(os.Stderr, "[SLD] Running Node %s: %s\n", nodeVersion, formatCommand(command))
	code, err := runAttached(command, func(ctx context.Context, opts docker.ExecOptions) (int, error) {
		return executor.RunEphemeral(ctx, desired, opts)
	})
	return exitWith(code, err)
}

// runAttached wires the current process's stdio to a container command. A TTY
// is allocated when stdin and stdout are both terminals; piped stdin
// (e.g. `sld v2 exec mysql < dump.sql`) is forwarded without a TTY.
func runAttached(command []string, run func(context.Context, docker.ExecOptions) (int, error)) (int, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	inFd, inIsTerm := term.GetFdInfo(os.Stdin)
	outFd, outIsTerm := term.GetFdInfo(os.Stdout)
	tty := inIsTerm && outIsTerm

	opts := docker.ExecOptions{
		Cmd:    command,
		TTY:    tty,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	switch {
	case tty:
		if ws, err := term.GetWinsize(outFd); err == nil && ws.Height > 0 && ws.Width > 0 {
			opts.ConsoleSize = &[2]uint{uint(ws.Height), uint(ws.Width)}
		}
		state, err := term.SetRawTerminal(inFd)
		if err != nil {
			return -1, fmt.Errorf("failed to configure terminal: %w", err)
		}
		defer term.RestoreTerminal(inFd, state)
		opts.Stdin = os.Stdin
		// In raw mode Ctrl-C is sent to the container, not to us.
		signal.Reset(os.Interrupt)
	case !inIsTerm:
		opts.Stdin = io.Reader(os.Stdin)
	}

	return run(ctx, opts)
}

// exitWith turns a container command result into the CLI's own result: a
// non-zero exit code terminates SLD with the same code, like `docker exec`.
func exitWith(code int, err error) error {
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// formatCommand formats a command slice for display.
func formatCommand(cmd []string) string {
	return strings.Join(cmd, " ")
}
