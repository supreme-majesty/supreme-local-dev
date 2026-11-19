package v2

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/moby/term"
	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// RegisterExecCommands adds exec, shell, and passthrough commands to the V2 subcommand.
func RegisterExecCommands(v2Cmd *cobra.Command) {
	// --- sld v2 shell ---
	var shellService string
	shellCmd := &cobra.Command{
		Use:   "shell [service|path]",
		Short: "Open an interactive shell in a project container (default: php)",
		Long: `Opens an interactive shell inside a running project container.
By default connects to the PHP container. Pass --service (e.g. --service db, --service nginx)
or specify a service name directly (php, nginx, db, redis, mailpit, node).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			svc := shellService
			if len(args) > 0 {
				if isKnownService(args[0]) {
					svc = args[0]
				} else {
					projectRoot = args[0]
				}
			}
			shellBin := []string{"bash"}
			if svc != docker.ServicePHP {
				shellBin = []string{"sh"}
			}
			return execInService(projectRoot, svc, shellBin)
		},
	}
	shellCmd.Flags().StringVarP(&shellService, "service", "s", docker.ServicePHP, "Target service container (php, nginx, db, redis, mailpit, node)")
	v2Cmd.AddCommand(shellCmd)

	// --- sld v2 exec ---
	var execService string
	execCmd := &cobra.Command{
		Use:                "exec [command...]",
		Short:              "Execute a command inside a project container (default: php)",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("usage: sld v2 exec [--service <svc>] <command> [args...]")
			}
			svc := docker.ServicePHP
			cmdArgs := args
			// Support optional leading --service / -s flag while keeping DisableFlagParsing for passthrough flags
			if len(cmdArgs) >= 3 && (cmdArgs[0] == "--service" || cmdArgs[0] == "-s") {
				svc = cmdArgs[1]
				cmdArgs = cmdArgs[2:]
			} else if len(cmdArgs) >= 2 && strings.HasPrefix(cmdArgs[0], "--service=") {
				svc = strings.TrimPrefix(cmdArgs[0], "--service=")
				cmdArgs = cmdArgs[1:]
			} else if len(cmdArgs) >= 2 && isKnownService(cmdArgs[0]) && cmdArgs[0] != "php" {
				// Allow "sld v2 exec db mysql ..." shorthand (keep "php" as command if someone runs "exec php -v")
				svc = cmdArgs[0]
				cmdArgs = cmdArgs[1:]
			}
			_ = execService
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInService(cwd, svc, cmdArgs)
		},
	}
	v2Cmd.AddCommand(execCmd)

	// --- sld v2 php ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "php [args...]",
		Short:              "Run PHP CLI inside the project PHP container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInService(cwd, docker.ServicePHP, append([]string{"php"}, args...))
		},
	})

	// --- sld v2 composer ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "composer [args...]",
		Short:              "Run Composer inside the PHP container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInService(cwd, docker.ServicePHP, append([]string{"composer"}, args...))
		},
	})

	// --- sld v2 artisan ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "artisan [args...]",
		Short:              "Run Laravel Artisan inside the PHP container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInService(cwd, docker.ServicePHP, append([]string{"php", "artisan"}, args...))
		},
	})

	// --- sld v2 npm ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "npm [args...]",
		Short:              "Run npm via an ephemeral Node container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInNode(cwd, append([]string{"npm"}, args...))
		},
	})
}

func isKnownService(s string) bool {
	switch strings.ToLower(s) {
	case docker.ServicePHP, docker.ServiceNginx, docker.ServiceDB, docker.ServiceRedis, docker.ServiceNode, "mailpit":
		return true
	default:
		return false
	}
}

// execInService executes a command inside the specified project service
// container through the Docker API and exits with the command's exit code.
func execInService(projectRoot, service string, command []string) error {
	if service == "" {
		service = docker.ServicePHP
	}

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

	containers, err := engine.ListProjectContainers(context.Background(), identity.ID)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	targetName := ""
	for _, c := range containers {
		if strings.EqualFold(c.Service, service) && c.State == "running" {
			targetName = c.Name
			break
		}
	}
	if targetName == "" {
		return fmt.Errorf("service container %q is not running for project %s.\n  Run 'sld v2 start' first", service, identity.Slug)
	}

	executor, ok := engine.(docker.Executor)
	if !ok {
		return fmt.Errorf("docker engine does not support exec")
	}

	fmt.Fprintf(os.Stderr, "[SLD] Executing in %s: %s\n", targetName, formatCommand(command))
	code, err := runAttached(command, func(ctx context.Context, opts docker.ExecOptions) (int, error) {
		return executor.Exec(ctx, targetName, opts)
	})
	return exitWith(code, err)
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

	nodeVersion := "22"
	if cfg, _, err := loadProjectConfig(projectRoot); err == nil && cfg.Node.Version != "" {
		nodeVersion = cfg.Node.Version
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
	if runtime.GOOS == "linux" {
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
