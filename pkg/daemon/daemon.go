// Package daemon holds the long-lived services behind the dashboard's
// database manager, logs, env editor, artisan console, healer and X-Ray
// pages. Project routing, services and plugins are handled by the V2
// control plane (internal/server); nothing here changes the host.
package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/supreme-majesty/supreme-local-dev/pkg/events"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

type Daemon struct {
	Events          *events.Bus
	XRayService     *services.XRayService
	DatabaseService *services.DatabaseService
	ProjectManager  *services.ProjectManager
	LogWatcher      *services.LogWatcher
	EnvManager      *services.EnvManager
	ArtisanService  *services.ArtisanService
	HealerService   *services.HealerService
	QueryWatcher    *services.QueryWatcher
	DockerDiscovery *services.DockerDiscoveryService
}

var instance *Daemon

// Initialize sets up the global instance.
func Initialize() (*Daemon, error) {
	if instance != nil {
		return instance, nil
	}

	eventBus := events.NewBus()
	xrayService := services.NewXRayService(eventBus)
	databaseService := services.NewDatabaseService()
	if err := services.ApplyPhpMyAdminHideConfig(); err != nil {
		log.Printf("Warning: phpMyAdmin hide_db config: %v", err)
	}

	// X-Ray's HTTP traffic now comes from the router's access log
	// (internal/server/xray.go); there is no host nginx log to tail.

	instance = &Daemon{
		Events:          eventBus,
		XRayService:     xrayService,
		DatabaseService: databaseService,
		ProjectManager:  services.NewProjectManager(workspaceDir()),
		LogWatcher:      services.NewLogWatcher(eventBus, func() map[string]string { return map[string]string{} }),
		EnvManager:      services.NewEnvManager(),
		ArtisanService:  services.NewArtisanService(eventBus),
		HealerService:   services.NewHealerService(eventBus),
		DockerDiscovery: services.NewDockerDiscoveryService(),
	}

	instance.HealerService.Start()

	// Queries on the configured MySQL server, shown on the X-Ray page. The
	// server's general log is on only while the page asks for it (Touch);
	// one left on by an earlier SLD is turned off and emptied.
	qw, err := services.NewQueryWatcher(eventBus, services.MySQLDSN())
	if err == nil {
		instance.QueryWatcher = qw
		go qw.Reset()
	} else {
		fmt.Printf("Warning: Failed to prepare QueryWatcher: %v\n", err)
	}

	return instance, nil
}

// GetClient returns the instance, creating it on first use.
func GetClient() (*Daemon, error) {
	if instance == nil {
		return Initialize()
	}
	return instance, nil
}

// workspaceDir is the folder the file browser starts in: the first
// workspace (SLD_WORKSPACES), /workspace in the SLD image, or the home folder.
func workspaceDir() string {
	if v := os.Getenv("SLD_WORKSPACES"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	if st, err := os.Stat("/workspace"); err == nil && st.IsDir() {
		return "/workspace"
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{"Developments", "Projects", "Sites", "code", "dev"} {
		if st, err := os.Stat(filepath.Join(home, d)); err == nil && st.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return home
}
