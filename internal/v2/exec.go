package v2

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
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
			return execInService(projectRoot, svc, shellBin, true)
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
			return execInService(cwd, svc, cmdArgs, false)
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
			return execInService(cwd, docker.ServicePHP, append([]string{"php"}, args...), false)
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
			return execInService(cwd, docker.ServicePHP, append([]string{"composer"}, args...), false)
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
			return execInService(cwd, docker.ServicePHP, append([]string{"php", "artisan"}, args...), false)
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

// execInService executes a command inside the specified project service container.
func execInService(projectRoot, service string, command []string, interactive bool) error {
	if service == "" {
		service = docker.ServicePHP
	}

	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	engine, err := docker.NewDockerEngine()
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer engine.Close()

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

	// Use docker exec via the Docker CLI for full TTY support.
	execArgs := []string{"exec"}
	if interactive {
		execArgs = append(execArgs, "-it")
	}
	execArgs = append(execArgs, targetName)
	execArgs = append(execArgs, command...)

	fmt.Printf("[SLD] Executing in %s: %s\n", targetName, formatCommand(command))

	return runDockerCLI(execArgs...)
}

// execInNode runs a command in an ephemeral Node container mounted to the project.
func execInNode(projectRoot string, command []string) error {
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	nodeVersion := "22"
	cfg, _, err := loadProjectConfig(projectRoot)
	if err == nil && cfg.Node.Version != "" {
		nodeVersion = cfg.Node.Version
	}

	nodeImage := fmt.Sprintf("node:%s-bookworm-slim", nodeVersion)
	networkName := identity.NetworkName()

	execArgs := []string{"run", "--rm", "-it"}
	// Preserve host UID/GID on Linux so node_modules/build outputs are not root-owned
	if runtime.GOOS == "linux" {
		execArgs = append(execArgs, "-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	}
	// Attach to project network when running
	if engine, err := docker.NewDockerEngine(); err == nil {
		containers, listErr := engine.ListProjectContainers(context.Background(), identity.ID)
		_ = engine.Close()
		if listErr == nil && len(containers) > 0 {
			execArgs = append(execArgs, "--network", networkName)
		}
	}
	execArgs = append(execArgs,
		"-v", fmt.Sprintf("%s:/app", projectRoot),
		"-w", "/app",
		nodeImage,
	)
	execArgs = append(execArgs, command...)

	fmt.Printf("[SLD] Running Node %s: %s\n", nodeVersion, formatCommand(command))

	return runDockerCLI(execArgs...)
}

// runDockerCLI shells out to the docker CLI binary for TTY-attached exec.
func runDockerCLI(args ...string) error {
	cmd := newCommand("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// formatCommand formats a command slice for display.
func formatCommand(cmd []string) string {
	return strings.Join(cmd, " ")
}
