package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/api"
)

var rootCmd = &cobra.Command{
	Use:   "sld",
	Short: "Supreme Local Dev",
	Long:  `High-performance local development environment for PHP/Laravel.`,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install SLD dependencies and core services",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Installing Supreme Local Dev...")
		
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		if err := d.EnsureInstalled(); err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}

		fmt.Println("Supreme Local Dev installed successfully! 🚀")
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of services",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil {
			return err
		}

		// Simple status check
		running, err := d.Adapter.IsServiceRunning("nginx")
		status := "STOPPED"
		if err == nil && running {
			status = "RUNNING"
		}
		
		fmt.Printf("Nginx: %s\n", status)
		fmt.Printf("PHP:   %s\n", d.Adapter.GetPHPVersion())
		
		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(statusCmd)
	
	// Project Management Commands
	rootCmd.AddCommand(parkCmd)
	rootCmd.AddCommand(forgetCmd)
	rootCmd.AddCommand(pathsCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
	rootCmd.AddCommand(linksCmd)
	rootCmd.AddCommand(secureCmd)
	rootCmd.AddCommand(phpCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(guiCmd)
}

// --- Commands ---

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the SLD API server and dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure core is installed/ready?
		_, err := daemon.GetClient()
		if err != nil { return err }
		
		// Start Server
		srv := api.NewServer(2025)
		return srv.Start()
	},
}

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Open the SLD Dashboard in your browser",
	RunE: func(cmd *cobra.Command, args []string) error {
		url := "http://localhost:2025"
		fmt.Printf("Opening %s...\n", url)
		
		// Linux xdg-open
		exec.Command("xdg-open", url).Start()
		// TODO: Support Mac open, Windows start
		
		return nil
	},
}

var phpCmd = &cobra.Command{
	Use:   "php [version]",
	Short: "Switch global PHP version (e.g. 8.1, 8.2)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("please specify a PHP version (e.g. 8.2)")
		}
		version := args[0]
		
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		return d.SwitchPHP(version)
	},
}

var secureCmd = &cobra.Command{
	Use:   "secure",
	Short: "Enable HTTPS (installs mkcert and updates config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		return d.Secure()
	},
}

var parkCmd = &cobra.Command{
	Use:   "park [path]",
	Short: "Register a directory to serve projects from",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		if err := d.Park(path); err != nil {
			return err
		}
		fmt.Printf("Parked directory: %s\n", path)
		return nil
	},
}

var forgetCmd = &cobra.Command{
	Use:   "forget [path]",
	Short: "Remove a directory from parked paths",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		if err := d.Forget(path); err != nil {
			return err
		}
		fmt.Printf("Forgot directory: %s\n", path)
		return nil
	},
}

var pathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "List all parked directories",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		fmt.Println("Parked Paths:")
		for _, p := range d.State.Data.Paths {
			fmt.Printf(" - %s\n", p)
		}
		return nil
	},
}

var linkCmd = &cobra.Command{
	Use:   "link [name]",
	Short: "Link the current directory to a domain",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := os.Getwd()
		name := ""
		
		if len(args) > 0 {
			name = args[0]
		} else {
			name = filepath.Base(path)
		}
		
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		if err := d.Link(name, path); err != nil {
			return err
		}
		fmt.Printf("Linked http://%s.test to %s\n", name, path)
		return nil
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink [name]",
	Short: "Remove a link",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			cwd, _ := os.Getwd()
			name = filepath.Base(cwd)
		}
		
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		if err := d.Unlink(name); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s\n", name)
		return nil
	},
}

var linksCmd = &cobra.Command{
	Use:   "links",
	Short: "List all linked sites",
	RunE: func(cmd *cobra.Command, args []string) error {
		d, err := daemon.GetClient()
		if err != nil { return err }
		
		fmt.Println("Linked Sites:")
		for name, path := range d.State.Data.Links {
			fmt.Printf(" - %s -> %s\n", name, path)
		}
		return nil
	},
}
