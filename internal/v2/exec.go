package v2

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/project"
)

// RegisterExecCommands adds exec, shell, and passthrough commands to the V2 subcommand.
func RegisterExecCommands(v2Cmd *cobra.Command) {
	// --- sld v2 shell ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:   "shell [path]",
		Short: "Open an interactive shell in the PHP container",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := resolveProjectRoot(args)
			if err != nil {
				return err
			}
			return execInPHP(projectRoot, []string{"bash"}, true)
		},
	})

	// --- sld v2 exec ---
	v2Cmd.AddCommand(&cobra.Command{
		Use:                "exec [command...]",
		Short:              "Execute a command inside the PHP container",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("usage: sld v2 exec <command> [args...]")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return execInPHP(cwd, args, false)
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
			return execInPHP(cwd, append([]string{"composer"}, args...), false)
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
			return execInPHP(cwd, append([]string{"php", "artisan"}, args...), false)
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

// execInPHP executes a command inside the project's PHP container.
func execInPHP(projectRoot string, command []string, interactive bool) error {
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	engine, err := docker.NewDockerEngine()
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer engine.Close()

	phpContainer := identity.PHPContainerName()

	// Check if the container is running
	containers, err := engine.ListProjectContainers(context.Background(), identity.ID)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	found := false
	for _, c := range containers {
		if c.Name == phpContainer && c.State == "running" {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("PHP container %s is not running.\n  Run 'sld v2 start' first", phpContainer)
	}

	// Use docker exec via the Docker CLI for TTY support.
	// The SDK's ContainerExecCreate/Start doesn't handle TTY well without raw terminal setup.
	execArgs := []string{"exec"}
	if interactive {
		execArgs = append(execArgs, "-it")
	}
	execArgs = append(execArgs, phpContainer)
	execArgs = append(execArgs, command...)

	fmt.Printf("[SLD] Executing in %s: %s\n", phpContainer, formatCommand(command))

	return runDockerCLI(execArgs...)
}

// execInNode runs a command in an ephemeral Node container mounted to the project.
func execInNode(projectRoot string, command []string) error {
	identity, err := project.ComputeIdentity(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to compute project identity: %w", err)
	}

	// Load config to get Node version if specified
	nodeVersion := "22"
	cfg, _, err := loadProjectConfig(projectRoot)
	if err == nil && cfg.Node.Version != "" {
		nodeVersion = cfg.Node.Version
	}

	nodeImage := fmt.Sprintf("node:%s-bookworm-slim", nodeVersion)
	networkName := identity.NetworkName()

	// Ephemeral container: --rm, mounted to /app, attached to project network
	execArgs := []string{
		"run", "--rm", "-it",
		"--network", networkName,
		"-v", fmt.Sprintf("%s:/app", projectRoot),
		"-w", "/app",
		nodeImage,
	}
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
	result := ""
	for i, part := range cmd {
		if i > 0 {
			result += " "
		}
		result += part
	}
	return result
}
