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
  2. Checks host port 80/443 availability
  3. Creates the sld-global bridge network and wildcard certificates
  4. Starts the Traefik router container for *.test domain routing`,
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
containers for the project in dependency order with health verification.`,
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
		Short: "Stop and remove Docker containers for the current project (preserves volumes)",
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

	// --- sld v2 destroy ---
	var destroyVolumes bool
	var destroyYes bool
	destroyCmd := &cobra.Command{
		Use:   "destroy [path]",
		Short: "Tear down project containers and network (add --volumes to purge data)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Destroy(context.Background(), projectRoot, destroyVolumes, destroyYes)
		},
	}
	destroyCmd.Flags().BoolVar(&destroyVolumes, "volumes", false, "Also delete persistent database and Redis volumes")
	destroyCmd.Flags().BoolVarP(&destroyYes, "yes", "y", false, "Skip interactive confirmation prompt")
	v2Cmd.AddCommand(destroyCmd)

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
	var statusAll bool
	var statusJSON bool
	statusCmd := &cobra.Command{
		Use:   "status [path]",
		Short: "Show status of Docker containers for the current project (or --all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Status(context.Background(), projectRoot, statusAll, statusJSON)
		},
	}
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "Show status of all SLD V2 managed projects")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output status as JSON")
	v2Cmd.AddCommand(statusCmd)

	// --- sld v2 inspect ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "inspect [path]",
		Short: "Output detailed JSON representation of the project's Docker state",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				return err
			}
			return runner.Inspect(context.Background(), projectRoot)
		},
	})

	// --- sld v2 secure ---
	var secureTLD string
	secureCmd := &cobra.Command{
		Use:   "secure",
		Short: "Generate wildcard TLS certificates via mkcert and reload the router",
		RunE: func(cmd *cobra.Command, args []string) error {
			runner, err := lazyRunner()
			if err != nil {
				// Allow certificate generation even if Docker isn't running yet
				runner = &Runner{}
			}
			return runner.Secure(context.Background(), secureTLD)
		},
	}
	secureCmd.Flags().StringVar(&secureTLD, "tld", "test", "Top-level domain for wildcard certificate")
	v2Cmd.AddCommand(secureCmd)

	// --- sld v2 config ---
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Validate or inspect .sld/config.yaml",
	}
	configCmd.AddCommand(&cobra.Command{
		Use:   "validate [path]",
		Short: "Validate .sld/config.yaml syntax and runtime constraints",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			return ConfigValidate(projectRoot)
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "show [path]",
		Short: "Print resolved .sld/config.yaml with defaults applied",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			return ConfigShow(projectRoot)
		},
	})
	v2Cmd.AddCommand(configCmd)

	// --- sld v2 doctor ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check Docker environment health, ports, TLS, and DNS",
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
Optionally specify a service: php, nginx, db, redis, mailpit, node.`,
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

	// --- sld v2 migrate-v1 (backward-compatible alias) ---
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
				runner = &Runner{}
			}
			return runner.MigrateV1(context.Background(), projectRoot)
		},
	})

	// --- sld v2 legacy ---
	legacyCmd := &cobra.Command{
		Use:   "legacy",
		Short: "Inspect, migrate, or remove legacy SLD V1 host setup",
	}
	legacyCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Inspect host for V1 installations, daemon service, and configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&Runner{}).LegacyStatus(context.Background())
		},
	})
	var migrateExecute bool
	legacyMigrateCmd := &cobra.Command{
		Use:   "migrate [path]",
		Short: "Migrate a V1 host project and database to V2 Docker (defaults to dry-run)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			runner, err := lazyRunner()
			if err != nil {
				runner = &Runner{}
			}
			return runner.LegacyMigrate(context.Background(), projectRoot, migrateExecute)
		},
	}
	legacyMigrateCmd.Flags().BoolVar(&migrateExecute, "execute", false, "Apply migration changes (omit for --dry-run preview)")
	legacyCmd.AddCommand(legacyMigrateCmd)

	var legacyRemoveYes bool
	legacyRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Archive V1 state and stop/disable V1 systemd daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return (&Runner{}).LegacyRemove(context.Background(), legacyRemoveYes)
		},
	}
	legacyRemoveCmd.Flags().BoolVarP(&legacyRemoveYes, "yes", "y", false, "Skip interactive confirmation prompt")
	legacyCmd.AddCommand(legacyRemoveCmd)
	v2Cmd.AddCommand(legacyCmd)

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
