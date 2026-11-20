package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	v2 "github.com/supreme-majesty/supreme-local-dev/internal/v2"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "sld",
	Short: "Supreme Local Dev",
	Long: `Supreme Local Dev runs your projects (PHP, Node, Python, Ruby, Go, static,
Dockerfile or Docker Compose) in containers, each at its own <name>.localhost.

Most people run SLD as a container and use the dashboard ('sld serve').
Without the container, these commands manage the project in the current folder:

  sld init            one-time setup: router, network and HTTPS certificate authority
  sld scan            find projects in a folder and write their .sld/config.yaml
  sld start / stop    start or stop the project in the current folder
  sld status          what is running
  sld logs / shell    look inside a running project`,
	Version: Version,
	// main prints the error once; don't repeat it or dump usage on runtime failures.
	SilenceUsage:  true,
	SilenceErrors: true,
}

// --- Database management ---

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management commands",
}

var dbCloneCmd = &cobra.Command{
	Use:   "clone <source> <target>",
	Short: "Clone a database",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		fmt.Printf("Cloning database %s -> %s...\n", args[0], args[1])
		if err := d.DatabaseService.CloneDatabase(args[0], args[1], services.CloneOptions{Mode: "both", CreateDatabase: true, AddAutoIncrement: true, AddConstraints: true}); err != nil {
			return err
		}
		fmt.Println("✅ Database cloned successfully!")
		return nil
	},
}

var dbSnapshotCmd = &cobra.Command{
	Use:   "snapshot <database> [table]",
	Short: "Create a snapshot of a database or table",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}
		table := ""
		if len(args) > 1 {
			table = args[1]
		}
		fmt.Printf("Creating snapshot of %s...\n", args[0])
		snapshot, err := d.DatabaseService.CreateSnapshot(args[0], table)
		if err != nil {
			return err
		}
		fmt.Printf("✅ Snapshot created: %s\n", snapshot.Filename)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(dbCloneCmd)
	dbCmd.AddCommand(dbSnapshotCmd)

	// Project commands: init, start, stop, status, logs, shell, scan, ...
	v2.RegisterCommands(rootCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
