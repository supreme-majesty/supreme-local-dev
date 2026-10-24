package v2

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
)

// lazyRunner creates a Runner on demand, connecting to Docker Engine.
func lazyRunner() (*Runner, error) {
	engine, err := docker.NewDockerEngine()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w\n  Is Docker Desktop running?", err)
	}
	return NewRunner(engine), nil
}

// RegisterCommands adds V2 Docker-backed subcommands to the root cobra command.
// These coexist with the legacy V1 host-based commands.
func RegisterCommands(rootCmd *cobra.Command) {
	v2Cmd := &cobra.Command{
		Use:   "v2",
		Short: "SLD V2 Docker-backed commands",
		Long:  "Docker-native project management commands for SLD V2. These use per-project containers instead of host-installed services.",
	}

	// --- sld v2 init ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize SLD V2 (create global network and start router)",
		Long: `Performs first-time SLD V2 setup:
  1. Validates Docker Engine connectivity
  2. Creates the sld-global bridge network
  3. Starts the Traefik router container for *.test domain routing`,
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Init(context.Background())
		},
	})

	// --- sld v2 start ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "start [path]",
		Short: "Start Docker containers for the current project",
		Long: `Reads .sld/config.yaml from the project root, resolves the PHP version,
database, Redis, Mailpit, and Node services, then starts isolated Docker
containers for the project. The Nginx sidecar is automatically registered
with Traefik for *.test domain routing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Start(context.Background(), projectRoot)
		},
	})

	// --- sld v2 stop ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "stop [path]",
		Short: "Stop and remove Docker containers for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Stop(context.Background(), projectRoot)
		},
	})

	// --- sld v2 restart ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "restart [path]",
		Short: "Restart Docker containers for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			ctx := context.Background()
			if err := runner.Stop(ctx, projectRoot); err != nil {
				return err
			}
			return runner.Start(ctx, projectRoot)
		},
	})

	// --- sld v2 status ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "status [path]",
		Short: "Show status of Docker containers for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Status(context.Background(), projectRoot)
		},
	})

	// --- sld v2 doctor ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check Docker environment health",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Doctor(context.Background())
		},
	})

	// --- sld v2 init-project ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "init-project [path]",
		Short: "Auto-detect project framework and generate .sld/config.yaml",
		Long: `Scans composer.json, package.json, and .env to detect the project type
(Laravel, Symfony, WordPress) and generates a .sld/config.yaml with
sensible defaults for PHP version, database, Redis, and Mailpit.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			return InitProject(projectRoot)
		},
	})

	// --- sld v2 logs ---
	var followLogs bool
	var tailLogs string
	logsCmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Tail logs of project Docker containers",
		Long: `Stream or view logs of containers for the current project.
Optionally specify a service: php, nginx, db, redis, mailpit.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			service := ""
			if len(args) > 0 {
				service = args[0]
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Logs(context.Background(), cwd, service, followLogs, tailLogs)
		},
	}
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVarP(&tailLogs, "tail", "n", "100", "Number of lines to show from the end of the logs")
	v2Cmd.AddCommand(logsCmd)

	// --- sld v2 open ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "open [path]",
		Short: "Open project domain in your default browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Open(context.Background(), projectRoot)
		},
	})

	// --- sld v2 update ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Pull the latest SLD V2 Docker images from GHCR",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Update(context.Background())
		},
	})

	// --- sld v2 migrate-v1 ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "migrate-v1 [path]",
		Short: "Migrate a project from legacy SLD V1 (host) to V2 (Docker)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.MigrateV1(context.Background(), projectRoot)
		},
	})

	// Exec, shell, and passthrough commands
	RegisterExecCommands(v2Cmd)

	rootCmd.AddCommand(v2Cmd)
}

// resolveProjectRoot determines the project root from args or current directory.
func resolveProjectRoot(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine current directory: %w", err)
	}
	return cwd, nil
}
