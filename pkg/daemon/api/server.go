package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/pkg/assets"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/services"
)

type Server struct {
	Port  int
	roots []string // folders that path-taking routes may touch; see WithRoots
}

func NewServer(port int) *Server {
	return &Server{Port: port}
}

// Mux builds the API and dashboard routes without starting a listener, so
// `sld serve` can add its own routes and middleware around them.
func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)

	// Database Manager
	mux.HandleFunc("/api/db/status", s.handleDBStatus)
	mux.HandleFunc("/api/db/driver", s.handleDBDriver)
	mux.HandleFunc("/api/db/sources", s.handleDBSources)
	mux.HandleFunc("/api/db/catalog", s.handleDBCatalog)
	mux.HandleFunc("/api/db/target", s.handleDBTarget)
	mux.HandleFunc("/api/db/databases", s.handleDBDatabases)
	mux.HandleFunc("/api/db/create", s.handleDBCreate)
	mux.HandleFunc("/api/db/delete", s.handleDBDelete)
	mux.HandleFunc("/api/db/rename", s.handleDBRename)
	mux.HandleFunc("/api/db/tables", s.handleDBTables)
	mux.HandleFunc("/api/db/table", s.handleDBTableData)
	mux.HandleFunc("/api/db/schema", s.handleDBSchema)
	mux.HandleFunc("/api/db/relationships", s.handleDBRelationships)
	mux.HandleFunc("/api/db/indexes", s.handleDBIndexes)
	mux.HandleFunc("/api/db/snapshots", s.handleDBSnapshots)
	mux.HandleFunc("/api/db/snapshots/download", s.handleDBDownload)
	mux.HandleFunc("/api/db/snapshots/restore", s.handleDBRestore)
	mux.HandleFunc("/api/db/restore", s.handleDBRestore)
	mux.HandleFunc("/api/db/import", s.handleDBImport)
	mux.HandleFunc("/api/db/query", s.handleDBQuery)
	mux.HandleFunc("/api/db/transaction", s.handleDBTransaction)
	mux.HandleFunc("/api/db/import/analyze", s.handleDBImportAnalyze)
	mux.HandleFunc("/api/db/import/execute", s.handleDBImportExecute)
	mux.HandleFunc("/api/db/import/smart", s.handleDBImportSmart)
	mux.HandleFunc("/api/db/seed", s.handleDBSeed)
	mux.HandleFunc("/api/db/stats", s.handleDBStats)
	mux.HandleFunc("/api/db/explain", s.handleDBExplain)
	mux.HandleFunc("/api/db/full-schema", s.handleDBFullSchema)
	mux.HandleFunc("/api/db/clone", s.handleDBClone)
	mux.HandleFunc("/api/db/rewind", s.handleDBRewind)
	mux.HandleFunc("/api/db/maintenance", s.handleDBMaintenance)
	mux.HandleFunc("/api/db/search", s.handleDBSearch)
	mux.HandleFunc("/api/db/foreign-values", s.handleDBForeignValues)
	mux.HandleFunc("/api/db/collations", s.handleDBCollations)
	mux.HandleFunc("/api/db/settings", s.handleDBSettings)
	mux.HandleFunc("/api/db/migrations", s.handleDBMigrations)
	mux.HandleFunc("/api/db/migrations/init", s.handleDBMigrationsInit)
	mux.HandleFunc("/api/db/migrations/create", s.handleDBMigrationsCreate)
	mux.HandleFunc("/api/db/migrations/run", s.handleDBMigrationsRun)
	mux.HandleFunc("/api/db/compare", s.handleDBCompare)
	mux.HandleFunc("/api/db/optimize", s.handleDBOptimize)
	mux.HandleFunc("/api/db/pii/scan", s.handleDBPIIScan)
	mux.HandleFunc("/api/db/pii/mask", s.handleDBPIIMask)
	mux.HandleFunc("/api/db/query/analyze", s.handleDBQueryAnalyze)
	mux.HandleFunc("/api/db/docs", s.handleDBDocs)
	mux.HandleFunc("/api/db/snippets", s.handleDBSnippets)
	mux.HandleFunc("/api/db/audit", s.handleDBAudit)
	mux.HandleFunc("/api/db/profiles", s.handleDBProfiles)
	mux.HandleFunc("/api/db/webhooks", s.handleDBWebhooks)
	mux.HandleFunc("/api/db/maintenance/schedule", s.handleDBMaintenanceSchedule)
	mux.HandleFunc("/api/db/switch", s.handleDBSwitch)

	// Logging
	mux.HandleFunc("/api/logs/sources", s.handleLogSources)
	mux.HandleFunc("/api/logs/watch", s.handleLogWatch)
	mux.HandleFunc("/api/logs/unwatch", s.handleLogUnwatch)
	mux.HandleFunc("/api/xray/queries/watch", s.handleQueryWatch)

	// Supreme Healer
	mux.HandleFunc("/api/healer/issues", s.handleHealerIssues)
	mux.HandleFunc("/api/healer/resolve", s.handleHealerResolve)

	// Projects & System
	mux.HandleFunc("/api/projects/files", s.handleProjectFiles)
	mux.HandleFunc("/api/projects/file/content", s.handleProjectFileContent)
	mux.HandleFunc("/api/projects/file/save", s.handleProjectFileSave)
	mux.HandleFunc("/api/system/editors", s.handleSystemEditors)
	mux.HandleFunc("/api/system/open-editor", s.handleSystemOpenEditor)

	// Env Manager
	mux.HandleFunc("/api/env/files", s.handleEnvFiles)
	mux.HandleFunc("/api/env/read", s.handleEnvRead)
	mux.HandleFunc("/api/env/write", s.handleEnvWrite)
	mux.HandleFunc("/api/env/backups", s.handleEnvBackups)
	mux.HandleFunc("/api/env/restore", s.handleEnvRestore)

	// Artisan Runner
	mux.HandleFunc("/api/artisan/run", s.handleArtisanRun)
	mux.HandleFunc("/api/artisan/commands", s.handleArtisanCommands)

	// Initialize WebSocket Hub
	hub := NewHub()
	go hub.Run()
	SetupEventBridge(hub)
	mux.HandleFunc("/api/ws", s.handleWebSocket(hub))

	// Serve GUI static files
	guiFS, _ := assets.GetGuiFS()
	fileServer := http.FileServer(guiFS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Skip API requests (addressed by specific handlers, but safe to check)
		if strings.HasPrefix(path, "/api") {
			http.NotFound(w, r)
			return
		}

		f, err := guiFS.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			defer f.Close()
			stat, _ := f.Stat()
			if !stat.IsDir() {
				if strings.HasPrefix(path, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
					w.Header().Set("Pragma", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	return mux
}

// Responses
type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func jsonResponse(w http.ResponseWriter, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// Projects & System

func (s *Server) handleProjectFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path is required"}, 400)
		return
	}
	if !s.allowPath(w, path) {
		return
	}

	d, _ := daemon.GetClient()
	files, err := d.ProjectManager.ListFiles(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, files, 200)
}

func (s *Server) handleProjectFileContent(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path is required"}, 400)
		return
	}
	if !s.allowPath(w, path) {
		return
	}

	d, _ := daemon.GetClient()
	content, err := d.ProjectManager.ReadFile(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, map[string]string{"content": content}, 200)
}

func (s *Server) handleProjectFileSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	if !s.allowPath(w, req.Path) {
		return
	}

	d, _ := daemon.GetClient()
	if err := d.ProjectManager.SaveFile(req.Path, req.Content); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, map[string]string{"message": "File saved successfully"}, 200)
}

func (s *Server) handleSystemEditors(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	editors := d.ProjectManager.DetectEditors()
	if editors == nil {
		editors = make([]services.Editor, 0)
	}
	jsonResponse(w, editors, 200)
}

func (s *Server) handleSystemOpenEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}
	var req struct {
		Path   string `json:"path"`
		Editor string `json:"editor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	if !s.allowPath(w, req.Path) {
		return
	}

	d, _ := daemon.GetClient()
	if err := d.ProjectManager.OpenInEditor(req.Path, req.Editor); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

// Plugins

// Tunnel / Share Handlers

// Database Manager Handlers

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, 200)
}

func (s *Server) handleDBStatus(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	// Verify connection status without leaking connection pools on repeated polls
	err := d.DatabaseService.StatusCheck()

	cfg := d.DatabaseService.ActiveConfig()
	host := cfg.Host
	port := cfg.Port
	user := cfg.User

	if host == "" {
		if d.DatabaseService.DriverName() == "postgres" {
			host = services.PostgresHost()
			port = services.PostgresPort()
			user = services.PostgresUser()
		} else {
			host = services.MySQLHost()
			port = services.MySQLPort()
			user = services.MySQLUser()
		}
	}

	status := map[string]interface{}{
		"connected":     err == nil,
		"host":          host,
		"port":          port,
		"user":          user,
		"driver":        d.DatabaseService.DriverName(),
		"active_target": d.DatabaseService.ActiveTarget(),
	}
	if err != nil {
		status["error"] = err.Error()
	}
	jsonResponse(w, status, 200)
}

func (s *Server) handleDBSources(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()

	sources := []map[string]interface{}{}

	// 1. Default configured database (host / compose)
	defaultDriver := d.DatabaseService.DriverName()
	var defaultHost, defaultPort, defaultUser string
	if defaultDriver == "postgres" {
		defaultHost = services.PostgresHost()
		defaultPort = services.PostgresPort()
		defaultUser = services.PostgresUser()
	} else {
		defaultHost = services.MySQLHost()
		defaultPort = services.MySQLPort()
		defaultUser = services.MySQLUser()
	}

	sources = append(sources, map[string]interface{}{
		"id":        "default",
		"name":      fmt.Sprintf("Default (%s)", strings.ToUpper(defaultDriver)),
		"driver":    defaultDriver,
		"host":      defaultHost,
		"port":      defaultPort,
		"user":      defaultUser,
		"status":    "running",
		"is_active": d.DatabaseService.ActiveTarget() == "default",
		"type":      "default",
	})

	// 2. Discover Docker database containers (running first, then stopped)
	if d.DockerDiscovery != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		discovered, err := d.DockerDiscovery.DiscoverDatabaseContainers(ctx)
		cancel()
		if err == nil {
			var running, stopped []services.DiscoveredDatabase
			for _, db := range discovered {
				if db.Status == "running" {
					running = append(running, db)
				} else {
					stopped = append(stopped, db)
				}
			}
			isTestDB := func(db services.DiscoveredDatabase) bool {
				low := strings.ToLower(db.ContainerName + " " + db.Database)
				return strings.Contains(low, "_test") || strings.Contains(low, "-test") || strings.HasSuffix(low, "test")
			}
			sort.Slice(running, func(i, j int) bool {
				ti, tj := isTestDB(running[i]), isTestDB(running[j])
				if ti != tj {
					return !ti
				}
				return strings.ToLower(running[i].Name) < strings.ToLower(running[j].Name)
			})
			sort.Slice(stopped, func(i, j int) bool {
				return strings.ToLower(stopped[i].Name) < strings.ToLower(stopped[j].Name)
			})
			for _, db := range append(running, stopped...) {
				sources = append(sources, map[string]interface{}{
					"id":             db.ID,
					"name":           db.Name,
					"container_name": db.ContainerName,
					"driver":         db.Driver,
					"host":           db.Host,
					"port":           db.Port,
					"user":           db.User,
					"database":       db.Database,
					"status":         db.Status,
					"image":          db.Image,
					"is_active":      d.DatabaseService.ActiveTarget() == db.ID,
					"type":           "docker",
				})
			}
		}
	}

	jsonResponse(w, sources, 200)
}

func (s *Server) handleDBTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "POST method required"}, 405)
		return
	}

	var req struct {
		ID       string `json:"id"`
		Driver   string `json:"driver"`
		Host     string `json:"host"`
		Port     string `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if req.ID == "" || req.ID == "default" {
		if err := d.DatabaseService.SetDriver(req.Driver); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		jsonResponse(w, SuccessResponse{Success: true, Message: "Switched to default database"}, 200)
		return
	}

	var found *services.DiscoveredDatabase
	if d.DockerDiscovery != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		dbTarget, err := d.DockerDiscovery.FindDatabase(ctx, req.ID)
		cancel()
		if err == nil && dbTarget != nil {
			found = dbTarget
			if found.Status != "running" {
				jsonResponse(w, ErrorResponse{Error: fmt.Sprintf("container %s is %s — start it before connecting", found.Name, found.Status)}, 400)
				return
			}
			// If running inside Docker and target has no published port or shared network, attach to its network
			netCtx, netCancel := context.WithTimeout(context.Background(), 3*time.Second)
			resolvedHost, resolvedPort, _ := d.DockerDiscovery.EnsureNetworkAccess(netCtx, found)
			netCancel()

			if req.Driver == "" {
				req.Driver = found.Driver
			}
			if resolvedHost != "" {
				req.Host = resolvedHost
			} else if req.Host == "" {
				req.Host = found.Host
			}
			if resolvedPort != "" {
				req.Port = resolvedPort
			} else if req.Port == "" {
				req.Port = found.Port
			}
			if req.User == "" {
				req.User = found.User
			}
			if req.Password == "" {
				req.Password = found.Password
			}
		}
	}

	cfg := services.ConnectionConfig{
		Host:     req.Host,
		Port:     req.Port,
		User:     req.User,
		Password: req.Password,
	}

	if err := d.DatabaseService.SetTarget(req.ID, req.Driver, cfg); err != nil {
		// Automatic fallback to MYSQL_USER/MYSQL_PASSWORD if root login was rejected
		if found != nil && found.AltUser != "" && found.AltUser != cfg.User {
			altCfg := services.ConnectionConfig{
				Host:     cfg.Host,
				Port:     cfg.Port,
				User:     found.AltUser,
				Password: found.AltPassword,
			}
			if altErr := d.DatabaseService.SetTarget(req.ID, req.Driver, altCfg); altErr == nil {
				jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Connected to %s", found.Name)}, 200)
				return
			}
		}
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	label := req.ID
	if found != nil && found.Name != "" {
		label = found.Name
	}
	jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Connected to %s", label)}, 200)
}

func (s *Server) handleDBDriver(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	if r.Method == "GET" {
		jsonResponse(w, map[string]string{"driver": d.DatabaseService.DriverName()}, 200)
		return
	}
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "GET or POST method required"}, 405)
		return
	}
	var req struct {
		Driver string `json:"driver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	if err := d.DatabaseService.SetDriver(req.Driver); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, map[string]string{"driver": d.DatabaseService.DriverName()}, 200)
}

func (s *Server) handleDBDatabases(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	summaries, err := d.DatabaseService.ListDatabaseSummaries()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if summaries == nil {
		summaries = make([]services.DatabaseSummary, 0)
	}
	jsonResponse(w, summaries, 200)
}

// handleDBCatalog aggregates user databases across the default server and all running Docker DB containers
// so the sidebar and overview can list every dockerized database together with grouped aliases.
func (s *Server) handleDBCatalog(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	activeTarget := d.DatabaseService.ActiveTarget()

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		catalog = make([]services.CatalogDatabase, 0)
	)

	// Discover Docker DB containers first so we can query all running containers concurrently
	var discovered []services.DiscoveredDatabase
	if d.DockerDiscovery != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		dbs, err := d.DockerDiscovery.DiscoverDatabaseContainers(ctx)
		cancel()
		if err == nil {
			discovered = dbs
		}
	}

	// Track whether any running Docker DB already binds host 127.0.0.1:<defaultPort> to avoid duplicating "default"
	defaultDriver := d.DatabaseService.DriverName()
	defaultHost := services.MySQLHost()
	defaultPort := services.MySQLPort()
	defaultCfg := services.ConnectionConfig{}
	if defaultDriver == "postgres" {
		defaultHost = services.PostgresHost()
		defaultPort = services.PostgresPort()
	}

	dockerOwnsDefaultPort := false
	for _, db := range discovered {
		if db.Status == "running" && db.Host == "127.0.0.1" && db.Port == defaultPort && defaultHost == "127.0.0.1" {
			dockerOwnsDefaultPort = true
			break
		}
	}

	// 1. Default configured server (if reachable and not shadowed by a discovered host-port container)
	if !dockerOwnsDefaultPort {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Fast pre-check so an offline/missing default host doesn't stall catalog discovery for several seconds
			if defaultHost != "" && defaultPort != "" {
				conn, err := net.DialTimeout("tcp", net.JoinHostPort(defaultHost, defaultPort), 500*time.Millisecond)
				if err != nil {
					return
				}
				_ = conn.Close()
			}
			drv, err := d.DatabaseService.GetOrConnectSource("default", defaultDriver, defaultCfg, "", "")
			if err != nil {
				return
			}
			summaries, err := services.ListDatabaseSummariesWithDriver(drv)
			if err != nil {
				return
			}
			mu.Lock()
			for _, sum := range summaries {
				catalog = append(catalog, services.CatalogDatabase{
					Name:       sum.Name,
					Tables:     sum.Tables,
					SourceID:   "default",
					SourceName: fmt.Sprintf("Default (%s)", strings.ToUpper(defaultDriver)),
					SourceType: "default",
					Driver:     defaultDriver,
					Host:       defaultHost,
					Port:       defaultPort,
					Status:     "running",
					IsActive:   activeTarget == "default",
				})
			}
			mu.Unlock()
		}()
	}

	// 2. All running Docker database containers queried concurrently
	for _, db := range discovered {
		if db.Status != "running" {
			continue
		}
		target := db
		wg.Add(1)
		go func() {
			defer wg.Done()
			host, port := target.Host, target.Port
			if d.DockerDiscovery != nil {
				netCtx, netCancel := context.WithTimeout(context.Background(), 3*time.Second)
				resolvedHost, resolvedPort, _ := d.DockerDiscovery.EnsureNetworkAccess(netCtx, &target)
				netCancel()
				if resolvedHost != "" {
					host = resolvedHost
				}
				if resolvedPort != "" {
					port = resolvedPort
				}
			}

			cfg := services.ConnectionConfig{
				Host:     host,
				Port:     port,
				User:     target.User,
				Password: target.Password,
			}
			drv, err := d.DatabaseService.GetOrConnectSource(target.ID, target.Driver, cfg, target.AltUser, target.AltPassword)
			if err != nil {
				// Fallback to the container's declared env database name if live query failed
				if target.Database != "" && !services.IsSystemDatabase(target.Database) {
					mu.Lock()
					catalog = append(catalog, services.CatalogDatabase{
						Name:          target.Database,
						Tables:        0,
						SourceID:      target.ID,
						SourceName:    target.Name,
						SourceType:    "docker",
						ContainerName: target.ContainerName,
						Driver:        target.Driver,
						Host:          host,
						Port:          port,
						Status:        target.Status,
						IsActive:      activeTarget == target.ID,
					})
					mu.Unlock()
				}
				return
			}

			summaries, err := services.ListDatabaseSummariesWithDriver(drv)
			if err != nil {
				return
			}

			mu.Lock()
			for _, sum := range summaries {
				catalog = append(catalog, services.CatalogDatabase{
					Name:          sum.Name,
					Tables:        sum.Tables,
					SourceID:      target.ID,
					SourceName:    target.Name,
					SourceType:    "docker",
					ContainerName: target.ContainerName,
					Driver:        target.Driver,
					Host:          host,
					Port:          port,
					Status:        target.Status,
					IsActive:      activeTarget == target.ID,
				})
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	sort.Slice(catalog, func(i, j int) bool {
		if catalog[i].Name != catalog[j].Name {
			return catalog[i].Name < catalog[j].Name
		}
		return catalog[i].SourceName < catalog[j].SourceName
	})

	jsonResponse(w, catalog, 200)
}

func (s *Server) handleDBTables(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	if db == "" {
		jsonResponse(w, ErrorResponse{Error: "db parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	sourceID := r.URL.Query().Get("source")

	var (
		tables []services.TableInfo
		err    error
	)

	if sourceID != "" && sourceID != d.DatabaseService.ActiveTarget() {
		if sourceID == "default" {
			drv, cErr := d.DatabaseService.GetOrConnectSource("default", d.DatabaseService.DriverName(), services.ConnectionConfig{}, "", "")
			if cErr != nil {
				err = cErr
			} else {
				tables, err = drv.ListTables(db)
			}
		} else if d.DockerDiscovery != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			found, fErr := d.DockerDiscovery.FindDatabase(ctx, sourceID)
			cancel()
			if fErr != nil {
				err = fErr
			} else {
				netCtx, netCancel := context.WithTimeout(context.Background(), 3*time.Second)
				host, port, _ := d.DockerDiscovery.EnsureNetworkAccess(netCtx, found)
				netCancel()
				if host == "" {
					host = found.Host
				}
				if port == "" {
					port = found.Port
				}
				drv, cErr := d.DatabaseService.GetOrConnectSource(found.ID, found.Driver, services.ConnectionConfig{
					Host:     host,
					Port:     port,
					User:     found.User,
					Password: found.Password,
				}, found.AltUser, found.AltPassword)
				if cErr != nil {
					err = cErr
				} else {
					tables, err = drv.ListTables(db)
				}
			}
		} else {
			tables, err = d.DatabaseService.ListTables(db)
		}
	} else {
		tables, err = d.DatabaseService.ListTables(db)
	}

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if tables == nil {
		tables = make([]services.TableInfo, 0)
	}
	jsonResponse(w, tables, 200)
}

func (s *Server) handleDBRelationships(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	if db == "" {
		jsonResponse(w, ErrorResponse{Error: "db parameter required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	rels, err := d.DatabaseService.GetTableRelationships(db)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if rels == nil {
		rels = make([]services.TableRelationship, 0)
	}
	jsonResponse(w, rels, 200)
}

func (s *Server) handleDBIndexes(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")
	if db == "" || table == "" {
		jsonResponse(w, ErrorResponse{Error: "db and table parameters required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	indexes, err := d.DatabaseService.GetTableIndexes(db, table)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if indexes == nil {
		indexes = make([]services.IndexInfo, 0)
	}
	jsonResponse(w, indexes, 200)
}

func (s *Server) handleDBCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "POST method required"}, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Name == "" {
		jsonResponse(w, ErrorResponse{Error: "database name is required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.CreateDatabase(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]string{"message": "Database created successfully"}, 200)
}

func (s *Server) handleDBDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" && r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "DELETE or POST method required"}, 405)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Name == "" {
		jsonResponse(w, ErrorResponse{Error: "database name is required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.DeleteDatabase(req.Name); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]string{"message": "Database deleted successfully"}, 200)
}

func (s *Server) handleDBRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "POST method required"}, 405)
		return
	}

	var req struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.OldName == "" || req.NewName == "" {
		jsonResponse(w, ErrorResponse{Error: "old_name and new_name are required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.RenameDatabase(req.OldName, req.NewName); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]string{"message": "Database renamed successfully"}, 200)
}

func (s *Server) handleDBTableData(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")
	if db == "" || table == "" {
		jsonResponse(w, ErrorResponse{Error: "db and table parameters required"}, 400)
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// New parameters: sort, order, profile
	sortCol := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order") // ASC or DESC
	profile := r.URL.Query().Get("profile") == "true"

	// Convert offset/limit to page/perPage
	perPage := limit
	page := (offset / limit) + 1

	d, _ := daemon.GetClient()
	data, err := d.DatabaseService.GetTableDataEx(db, table, page, perPage, sortCol, sortOrder, profile)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	cols := data.Columns
	if cols == nil {
		cols = make([]services.ColumnInfo, 0)
	}
	rows := data.Rows
	if rows == nil {
		rows = make([]map[string]interface{}, 0)
	}

	resp := map[string]interface{}{
		"columns":     cols,
		"rows":        rows,
		"total":       data.Total,
		"page":        data.Page,
		"per_page":    data.PerPage,
		"limit":       data.PerPage,
		"offset":      (data.Page - 1) * data.PerPage,
		"total_pages": data.TotalPages,
	}

	// Include query time if profiling was enabled
	if profile && data.QueryTime > 0 {
		resp["query_time"] = data.QueryTime
	}

	jsonResponse(w, resp, 200)
}

func (s *Server) handleDBSchema(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	table := r.URL.Query().Get("table")
	if db == "" || table == "" {
		jsonResponse(w, ErrorResponse{Error: "db and table parameters required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	schema, err := d.DatabaseService.GetTableColumns(db, table)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if schema == nil {
		schema = make([]services.ColumnInfo, 0)
	}
	jsonResponse(w, schema, 200)
}

func (s *Server) handleDBSnapshots(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()

	switch r.Method {
	case "GET":
		db := r.URL.Query().Get("db")
		snapshots, err := d.DatabaseService.ListSnapshots()
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		// Filter by DB if requested and map to frontend format
		response := make([]map[string]interface{}, 0)
		for _, s := range snapshots {
			if db != "" && s.Database != db {
				continue
			}
			response = append(response, map[string]interface{}{
				"id":         s.Filename, // use filename as ID
				"filename":   s.Filename,
				"database":   s.Database,
				"table":      s.Table,
				"size":       s.Size,
				"created_at": s.CreatedAt,
			})
		}
		jsonResponse(w, response, 200)

	case "POST":
		var req struct {
			Database string `json:"database"`
			Table    string `json:"table"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}

		snapshot, err := d.DatabaseService.CreateSnapshot(req.Database, req.Table)
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		resp := map[string]interface{}{
			"id":         snapshot.Filename,
			"filename":   snapshot.Filename,
			"database":   snapshot.Database,
			"table":      snapshot.Table,
			"size":       snapshot.Size,
			"created_at": snapshot.CreatedAt,
		}
		jsonResponse(w, resp, 200)

	case "DELETE":
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}

		if req.ID == "" {
			jsonResponse(w, ErrorResponse{Error: "snapshot id is required"}, 400)
			return
		}

		// ID is filename
		if err := d.DatabaseService.DeleteSnapshot(req.ID); err != nil {
			fmt.Printf("Error deleting snapshot %s: %v\n", req.ID, err)
			jsonResponse(w, ErrorResponse{Error: fmt.Sprintf("Failed to delete snapshot: %v", err)}, 500)
			return
		}
		jsonResponse(w, SuccessResponse{Success: true}, 200)
	}
}

func (s *Server) handleDBRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string `json:"database"`
		Path     string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	// RestoreSnapshot takes filename (Path field from frontend)
	if err := d.DatabaseService.RestoreSnapshot(req.Path); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true, Message: "Database restored successfully"}, 200)
}

func (s *Server) handleDBQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string `json:"database"`
		Query    string `json:"query"`
		TxID     string `json:"tx_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	result, err := d.DatabaseService.ExecuteQuery(req.Database, req.Query, req.TxID)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, result, 200)
}

func (s *Server) handleDBTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database  string `json:"database"`
		Operation string `json:"operation"` // begin, commit, rollback
		TxID      string `json:"tx_id"`     // only for commit/rollback
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	var txId string
	var err error

	switch strings.ToLower(req.Operation) {
	case "begin":
		txId, err = d.DatabaseService.BeginTransaction(req.Database)
	case "commit":
		err = d.DatabaseService.CommitTransaction(req.TxID)
	case "rollback":
		err = d.DatabaseService.RollbackTransaction(req.TxID)
	default:
		err = fmt.Errorf("invalid transaction operation: %s", req.Operation)
	}

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"success": true, "tx_id": txId}, 200)
}

func (s *Server) handleDBImportAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	const maxUploadSize = 50 << 20 // 50MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: "file is required"}, 400)
		return
	}
	defer file.Close()

	format := r.FormValue("format")
	if format == "" {
		// Detect from filename
		if strings.HasSuffix(strings.ToLower(handler.Filename), ".csv") {
			format = "csv"
		} else if strings.HasSuffix(strings.ToLower(handler.Filename), ".json") {
			format = "json"
		}
	}

	d, _ := daemon.GetClient()
	tempPath := filepath.Join(os.TempDir(), "sld_import_"+handler.Filename)

	dst, err := os.Create(tempPath)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	analysis, err := d.DatabaseService.AnalyzeImport(tempPath, format)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"analysis":  analysis,
		"temp_path": tempPath,
	}, 200)
}

func (s *Server) handleDBImportExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string            `json:"database"`
		Table    string            `json:"table"`
		TempPath string            `json:"temp_path"`
		Format   string            `json:"format"`
		Mapping  map[string]string `json:"mapping"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	err := d.DatabaseService.ExecuteImport(req.Database, req.Table, req.TempPath, req.Format, req.Mapping)

	// Clean up temp file
	os.Remove(req.TempPath)

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleDBImportSmart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string                   `json:"database"`
		Table    string                   `json:"table"`
		Mapping  map[string]string        `json:"mapping"`
		Data     []map[string]interface{} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Database == "" || req.Table == "" {
		jsonResponse(w, ErrorResponse{Error: "database and table are required"}, 400)
		return
	}

	if len(req.Data) == 0 {
		jsonResponse(w, ErrorResponse{Error: "no data to import"}, 400)
		return
	}

	// Build target columns from mapping
	var targetCols []string
	for _, target := range req.Mapping {
		if target != "" {
			targetCols = append(targetCols, target)
		}
	}

	if len(targetCols) == 0 {
		jsonResponse(w, ErrorResponse{Error: "no column mappings provided"}, 400)
		return
	}

	d, _ := daemon.GetClient()

	// Convert inline data to batch rows
	var batch [][]interface{}
	for _, item := range req.Data {
		row := make([]interface{}, len(targetCols))
		for i, target := range targetCols {
			// Find source key for this target
			var sourceKey string
			for s, t := range req.Mapping {
				if t == target {
					sourceKey = s
					break
				}
			}
			row[i] = item[sourceKey]
		}
		batch = append(batch, row)

		// Batch insert every 1000 rows
		if len(batch) >= 1000 {
			if err := d.DatabaseService.BatchInsert(req.Database, req.Table, targetCols, batch, ""); err != nil {
				jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
				return
			}
			batch = [][]interface{}{}
		}
	}

	// Final batch
	if len(batch) > 0 {
		if err := d.DatabaseService.BatchInsert(req.Database, req.Table, targetCols, batch, ""); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Imported %d rows", len(req.Data))}, 200)
}

func (s *Server) handleDBSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string            `json:"database"`
		Table    string            `json:"table"`
		Count    int               `json:"count"`
		Fakers   map[string]string `json:"fakers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Count <= 0 {
		jsonResponse(w, ErrorResponse{Error: "count must be positive"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	if err := d.DatabaseService.SeedData(req.Database, req.Table, req.Count, req.Fakers); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleDBStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	database := r.URL.Query().Get("database")
	if database == "" {
		jsonResponse(w, ErrorResponse{Error: "database is required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	stats, err := d.DatabaseService.GetStats(database)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, stats, 200)
}

func (s *Server) handleDBExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database string `json:"database"`
		Query    string `json:"query"`
		TxID     string `json:"tx_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	explanation, err := d.DatabaseService.ExplainQuery(req.Database, req.Query)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, explanation, 200)
}

func (s *Server) handleDBFullSchema(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()

	databases, err := d.DatabaseService.ListDatabases()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	schema := make(map[string][]string)
	for _, db := range databases {
		tables, err := d.DatabaseService.ListTables(db)
		if err == nil {
			tableNames := make([]string, len(tables))
			for i, t := range tables {
				tableNames[i] = t.Name
			}
			schema[db] = tableNames
		}
	}

	jsonResponse(w, schema, 200)
}

func (s *Server) handleDBClone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source         string `json:"source"`
		Target         string `json:"target"`
		Mode           string `json:"mode"`
		CreateDatabase bool   `json:"create_db"`
		AddDropTable   bool   `json:"add_drop"`
		AddAutoInc     bool   `json:"add_auto_inc"`
		AddConstraints bool   `json:"add_constraints"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Source == "" || req.Target == "" {
		jsonResponse(w, ErrorResponse{Error: "source and target database names required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	opts := services.CloneOptions{
		Mode:             req.Mode,
		CreateDatabase:   req.CreateDatabase,
		AddDropTable:     req.AddDropTable,
		AddAutoIncrement: req.AddAutoInc,
		AddConstraints:   req.AddConstraints,
	}
	if err := d.DatabaseService.CloneDatabase(req.Source, req.Target, opts); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: fmt.Sprintf("Database '%s' cloned to '%s'", req.Source, req.Target)}, 200)
}

// handleDBRewind performs a Time-Travel restore with auto-backup
func (s *Server) handleDBRewind(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Filename == "" {
		jsonResponse(w, ErrorResponse{Error: "filename required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	backup, err := d.DatabaseService.RewindDatabase(req.Filename)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Rewound to %s. Safety backup: %s", req.Filename, backup.Filename),
		"backup":  backup,
	}, 200)
}

func (s *Server) handleDBMaintenance(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Database  string   `json:"database"`
		Tables    []string `json:"tables"`
		Operation string   `json:"operation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.Database == "" || req.Operation == "" {
		jsonResponse(w, ErrorResponse{Error: "database and operation are required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	results, err := d.DatabaseService.Maintenance(req.Database, req.Tables, req.Operation)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"success": true, "results": results}, 200)
}

func (s *Server) handleDBSearch(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	q := r.URL.Query().Get("q")
	if db == "" || q == "" {
		jsonResponse(w, ErrorResponse{Error: "db and q parameters are required"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	results, err := d.DatabaseService.GlobalSearch(db, q)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, map[string]interface{}{"success": true, "results": results}, 200)
}

func (s *Server) handleDBDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id parameter required", 400)
		return
	}

	d, _ := daemon.GetClient()
	// Security: Sanitize ID to prevent directory traversal
	// In ListSnapshots we trust filenames in the dir, but here we take user input.
	// Simple check: must not contain slashes
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		http.Error(w, "invalid filename", 400)
		return
	}

	path := filepath.Join(d.DatabaseService.SnapDir, id)

	// Parse filename to extract db/table name for a clean download name
	// Formats: db_timestamp.sql or db__table_timestamp.sql
	cleanName := id
	baseName := strings.TrimSuffix(id, ".sql")
	if strings.Contains(baseName, "__") {
		// Table export: db__table_timestamp.sql -> table.sql
		parts := strings.Split(baseName, "__")
		if len(parts) >= 2 {
			remaining := parts[1]
			remainingParts := strings.Split(remaining, "_")
			if len(remainingParts) >= 2 {
				tableName := strings.Join(remainingParts[:len(remainingParts)-2], "_")
				cleanName = tableName + ".sql"
			}
		}
	} else {
		// Database export: db_timestamp.sql -> db.sql
		parts := strings.Split(baseName, "_")
		if len(parts) >= 2 {
			dbName := strings.Join(parts[:len(parts)-2], "_")
			cleanName = dbName + ".sql"
		}
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, cleanName))
	http.ServeFile(w, r, path)
}

func (s *Server) handleDBCollations(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	collations, err := d.DatabaseService.GetCollations()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if collations == nil {
		collations = make([]services.CollationInfo, 0)
	}
	jsonResponse(w, collations, 200)
}

func (s *Server) handleDBSettings(w http.ResponseWriter, r *http.Request) {
	db := r.URL.Query().Get("db")
	if db == "" {
		jsonResponse(w, ErrorResponse{Error: "Missing database name"}, 400)
		return
	}

	d, _ := daemon.GetClient()
	settings, err := d.DatabaseService.GetDatabaseSettings(db)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, settings, 200)
}

func (s *Server) handleDBImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonResponse(w, ErrorResponse{Error: "POST method required"}, 405)
		return
	}

	// 100MB limit with proper 413 response when exceeded
	const maxUploadSize = 100 << 20 // 100MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if err.Error() == "http: request body too large" {
			jsonResponse(w, ErrorResponse{Error: "File too large. Maximum size is 100MB"}, 413)
			return
		}
		jsonResponse(w, ErrorResponse{Error: "Error parsing form: " + err.Error()}, 400)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: "Error retrieving file"}, 400)
		return
	}
	defer file.Close()

	d, _ := daemon.GetClient()
	filename := filepath.Base(handler.Filename)
	shouldRestore := r.URL.Query().Get("restore") == "true"

	if shouldRestore {
		lowerName := strings.ToLower(filename)
		if !strings.HasSuffix(lowerName, ".sql") && !strings.HasSuffix(lowerName, ".gz") {
			jsonResponse(w, ErrorResponse{Error: "Only .sql and .sql.gz files are supported for SQL import"}, 400)
			return
		}

		dbName := r.FormValue("database")
		if dbName == "" {
			dbName = r.URL.Query().Get("database")
		}
		if dbName == "" {
			jsonResponse(w, ErrorResponse{Error: "database parameter required for SQL import"}, 400)
			return
		}

		tmpFile, err := os.CreateTemp("", "sld-sql-import-*-"+filename)
		if err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		destPath := tmpFile.Name()
		defer os.Remove(destPath)

		if _, err := io.Copy(tmpFile, file); err != nil {
			_ = tmpFile.Close()
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		// Flush and close before invoking mysql/psql so Windows/Linux handles see the complete file
		if err := tmpFile.Close(); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		if err := d.DatabaseService.ImportSQL(dbName, destPath); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}

		jsonResponse(w, SuccessResponse{
			Success: true,
			Message: fmt.Sprintf("Imported %s into %s successfully", filename, dbName),
		}, 200)
		return
	}

	// Snapshot upload only (restore=false)
	_ = os.MkdirAll(d.DatabaseService.SnapDir, 0755)
	destPath := filepath.Join(d.DatabaseService.SnapDir, filename)

	dst, err := os.Create(destPath)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if _, err := io.Copy(dst, file); err != nil {
		_ = dst.Close()
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	if err := dst.Close(); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "File uploaded successfully"}, 200)
}

func (s *Server) handleDBForeignValues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbName := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")
	column := r.URL.Query().Get("column")

	if dbName == "" || table == "" || column == "" {
		http.Error(w, "Missing database, table, or column parameter", http.StatusBadRequest)
		return
	}

	d, _ := daemon.GetClient()
	values, err := d.DatabaseService.GetForeignValues(dbName, table, column)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(values); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Log Management Handlers

func (s *Server) handleLogSources(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	sources := d.LogWatcher.GetAvailableSources()

	// Convert map to frontend friendly array
	type LogSourceInfo struct {
		ID    string `json:"id"`
		Path  string `json:"path"`
		Label string `json:"label"`
	}

	response := make([]LogSourceInfo, 0)
	for id, path := range sources {
		label := string(id)
		switch id {
		case services.LogSourceNginxError:
			label = "Nginx Error"
		case services.LogSourceNginxAccess:
			label = "Nginx Access"
		case services.LogSourcePHPFPM:
			label = "PHP-FPM"
		}

		response = append(response, LogSourceInfo{
			ID:    string(id),
			Path:  path,
			Label: label,
		})
	}

	jsonResponse(w, response, 200)
}

func (s *Server) handleLogWatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source      string `json:"source"`
		ProjectPath string `json:"project_path"` // Optional, for Laravel logs
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()

	var err error
	if strings.HasPrefix(req.Source, "laravel") && req.ProjectPath != "" {
		err = d.LogWatcher.WatchLaravelLog(req.ProjectPath)
	} else {
		err = d.LogWatcher.StartWatching(services.LogSource(req.Source))
	}

	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleLogUnwatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	d, _ := daemon.GetClient()
	d.LogWatcher.StopWatching(services.LogSource(req.Source))

	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

// handleQueryWatch is X-Ray's heartbeat while its Queries view is open: it
// keeps the database's query log on (POST) or turns it off now (DELETE).
func (s *Server) handleQueryWatch(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	if d == nil || d.QueryWatcher == nil {
		jsonResponse(w, ErrorResponse{Error: "no MySQL/MariaDB server is configured for SLD"}, 503)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if err := d.QueryWatcher.Touch(); err != nil {
			jsonResponse(w, ErrorResponse{Error: "could not watch queries: " + err.Error()}, 502)
			return
		}
	case http.MethodDelete:
		d.QueryWatcher.Stop()
	default:
		jsonResponse(w, ErrorResponse{Error: "use POST or DELETE"}, 405)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

// Env Manager Handlers

func (s *Server) handleEnvFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	projectPath := r.URL.Query().Get("project")
	if projectPath == "" {
		jsonResponse(w, ErrorResponse{Error: "project parameter required"}, 400)
		return
	}
	if !s.allowPath(w, projectPath) {
		return
	}

	d, _ := daemon.GetClient()
	files, err := d.EnvManager.ListEnvFiles(projectPath)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, files, 200)
}

func (s *Server) handleEnvRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path parameter required"}, 400)
		return
	}
	if !s.allowEnvFile(w, path) {
		return
	}

	d, _ := daemon.GetClient()
	envFile, err := d.EnvManager.ReadEnvFile(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, envFile, 200)
}

func (s *Server) handleEnvWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		return
	}

	var req struct {
		Path      string            `json:"path"`
		Variables map[string]string `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	if !s.allowEnvFile(w, req.Path) {
		return
	}

	d, _ := daemon.GetClient()
	if err := d.EnvManager.WriteEnvFile(req.Path, req.Variables); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "Env file saved with backup"}, 200)
}

func (s *Server) handleEnvBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		jsonResponse(w, ErrorResponse{Error: "path parameter required"}, 400)
		return
	}
	if !s.allowEnvFile(w, path) {
		return
	}

	d, _ := daemon.GetClient()
	backups, err := d.EnvManager.ListBackups(path)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, backups, 200)
}

func (s *Server) handleEnvRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		BackupPath string `json:"backup_path"`
		TargetPath string `json:"target_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	if !s.allowEnvBackup(w, req.BackupPath, req.TargetPath) {
		return
	}

	d, _ := daemon.GetClient()
	if err := d.EnvManager.RestoreBackup(req.BackupPath, req.TargetPath); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}

	jsonResponse(w, SuccessResponse{Success: true, Message: "Backup restored"}, 200)
}

// Artisan Runner Handlers

func (s *Server) handleArtisanRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		return
	}

	var req struct {
		ProjectPath string `json:"project_path"`
		Command     string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}

	if req.ProjectPath == "" || req.Command == "" {
		jsonResponse(w, ErrorResponse{Error: "project_path and command required"}, 400)
		return
	}
	if !s.allowPath(w, req.ProjectPath) {
		return
	}

	d, _ := daemon.GetClient()

	// Run async - output will stream via WebSocket
	go func() {
		if err := d.ArtisanService.RunCommand(req.ProjectPath, req.Command); err != nil {
			fmt.Printf("[ERROR] Artisan command failed: %v\n", err)
		}
	}()

	jsonResponse(w, SuccessResponse{Success: true, Message: "Command started"}, 202)
}

func (s *Server) handleArtisanCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		return
	}

	d, _ := daemon.GetClient()
	commands := d.ArtisanService.GetCommonCommands()
	jsonResponse(w, commands, 200)
}

// Service & Doctor Handlers

func (s *Server) handleDBMigrations(w http.ResponseWriter, r *http.Request) {
	database := r.URL.Query().Get("database")
	d, _ := daemon.GetClient()

	applied, _ := d.DatabaseService.GetAppliedMigrations(database)
	pending, _ := d.DatabaseService.GetPendingMigrations(database)

	jsonResponse(w, map[string]interface{}{
		"applied": applied,
		"pending": pending,
	}, 200)
}

func (s *Server) handleDBMigrationsInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database string `json:"database"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	d, _ := daemon.GetClient()
	if err := d.DatabaseService.InitializeMigrations(req.Database); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleDBMigrationsCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database string `json:"database"`
		Name     string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	d, _ := daemon.GetClient()
	filename, err := d.DatabaseService.CreateMigration(req.Database, req.Name)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, map[string]string{"filename": filename}, 200)
}

func (s *Server) handleDBMigrationsRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database string `json:"database"`
		Filename string `json:"filename"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	d, _ := daemon.GetClient()
	if err := d.DatabaseService.RunMigration(req.Database, req.Filename); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, SuccessResponse{Success: true}, 200)
}

func (s *Server) handleDBCompare(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	target := r.URL.Query().Get("target")
	d, _ := daemon.GetClient()

	diff, err := d.DatabaseService.CompareSchemas(source, target)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, diff, 200)
}

func (s *Server) handleDBOptimize(w http.ResponseWriter, r *http.Request) {
	database := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")
	d, _ := daemon.GetClient()

	suggestions, err := d.DatabaseService.GetOptimizationSuggestions(database, table)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, suggestions, 200)
}

func (s *Server) handleDBPIIScan(w http.ResponseWriter, r *http.Request) {
	database := r.URL.Query().Get("database")
	table := r.URL.Query().Get("table")
	d, _ := daemon.GetClient()

	results, err := d.DatabaseService.ScanPII(database, table)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, results, 200)
}

func (s *Server) handleDBPIIMask(w http.ResponseWriter, r *http.Request) {
	var config services.MaskingConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	d, _ := daemon.GetClient()
	if err := d.DatabaseService.AnonymizeTable(config); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "success"}, 200)
}

func (s *Server) handleDBQueryAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database string `json:"database"`
		Query    string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	d, _ := daemon.GetClient()
	plan, err := d.DatabaseService.AnalyzeQueryPlan(req.Database, req.Query)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, plan, 200)
}

func (s *Server) handleDBDocs(w http.ResponseWriter, r *http.Request) {
	database := r.URL.Query().Get("database")
	d, _ := daemon.GetClient()
	docs, err := d.DatabaseService.GenerateDocumentation(database)
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, docs, 200)
}

func (s *Server) handleDBSnippets(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	if r.Method == "POST" {
		var snippet services.QuerySnippet
		if err := json.NewDecoder(r.Body).Decode(&snippet); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}
		if err := d.DatabaseService.SaveSnippet(snippet); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		jsonResponse(w, map[string]string{"status": "success"}, 200)
		return
	}

	snippets, err := d.DatabaseService.GetSnippets()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, snippets, 200)
}

func (s *Server) handleDBAudit(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	logs, err := d.DatabaseService.GetAuditLog()
	if err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
		return
	}
	jsonResponse(w, logs, 200)
}

func (s *Server) handleDBProfiles(w http.ResponseWriter, r *http.Request) {
	d, _ := daemon.GetClient()
	if r.Method == "POST" {
		var profile services.ConnectionProfile
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
			return
		}
		if err := d.DatabaseService.SaveProfile(profile); err != nil {
			jsonResponse(w, ErrorResponse{Error: err.Error()}, 500)
			return
		}
		jsonResponse(w, map[string]string{"status": "success"}, 200)
		return
	}
	profiles, _ := d.DatabaseService.ListProfiles()
	jsonResponse(w, profiles, 200)
}

func (s *Server) handleDBWebhooks(w http.ResponseWriter, r *http.Request) {
	// Simple mock for now
	jsonResponse(w, []services.WebhookConfig{}, 200)
}

func (s *Server) handleDBMaintenanceSchedule(w http.ResponseWriter, r *http.Request) {
	// Simple mock for now
	jsonResponse(w, []services.MaintenanceTask{}, 200)
}

func (s *Server) handleDBSwitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, ErrorResponse{Error: err.Error()}, 400)
		return
	}
	// Logic to reconnect with profile credentials
	jsonResponse(w, map[string]string{"status": "connected"}, 200)
}
