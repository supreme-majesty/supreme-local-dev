package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/server"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon"
	"github.com/supreme-majesty/supreme-local-dev/pkg/daemon/api"
)

var (
	serveListen     string
	serveResetToken bool
)

// serveCmd runs the control plane: the dashboard, the database manager API
// and the V2 project API, behind token authentication. It is the SLD image's
// default command.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the SLD control plane (dashboard + API) with authentication",
	Long: `Serves the dashboard and API. On first start an admin token is printed once;
paste it on the dashboard's sign-in page. Projects are discovered in the
workspace folders (SLD_WORKSPACES, comma-separated; default /workspace inside
the SLD image, or the current folder on a host).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		engine, err := docker.NewDockerEngine()
		if err != nil {
			return fmt.Errorf("cannot connect to Docker: %w", err)
		}
		env, err := hostenv.Detect(ctx, engine)
		if err != nil {
			return fmt.Errorf("cannot inspect the SLD container (is the Docker socket mounted?): %w", err)
		}
		auth, err := server.LoadAuth(env.DataDir(), serveResetToken, os.Stdout)
		if err != nil {
			return err
		}
		auth.SetContainerName(env.ContainerName())
		fmt.Printf("[SLD] For a new admin token, run on the host: %s\n", auth.TokenCommand())

		workspaces, err := serveWorkspaces(env)
		if err != nil {
			return err
		}

		mux := api.NewServer(2025).WithRoots(workspaces).Mux() // dashboard, database manager, websocket
		controlPlane := server.New(server.Options{
			Engine: engine, Env: env, Auth: auth, Workspaces: workspaces, Version: Version,
		})
		controlPlane.Register(mux)
		if d, err := daemon.GetClient(); err == nil {
			controlPlane.StartXRay(context.Background(), d.Events) // X-Ray: requests through the router
		}

		// Automation: set up new projects and start the ones that should run.
		go func() {
			time.Sleep(3 * time.Second) // let the listener come up first
			controlPlane.RunStartupTasks(context.Background())
		}()

		listen := serveListen
		if listen == "" {
			listen = os.Getenv("SLD_LISTEN")
		}
		if listen == "" {
			listen = ":2025"
		}
		srv := &http.Server{
			Addr:              listen,
			Handler:           auth.Middleware(mux),
			ReadHeaderTimeout: 10 * time.Second,
		}

		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		fmt.Printf("[SLD] Control plane listening on %s (workspaces: %s)\n", listen, strings.Join(workspaces, ", "))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	},
}

// serveWorkspaces returns the folders scanned for projects.
func serveWorkspaces(env *hostenv.Env) ([]string, error) {
	var dirs []string
	if v := os.Getenv("SLD_WORKSPACES"); v != "" {
		for _, d := range strings.Split(v, ",") {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
	} else if env.InContainer() {
		dirs = []string{"/workspace"}
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dirs = []string{wd}
	}
	var out []string
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, err
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			fmt.Printf("[SLD] ⚠️  workspace %s is not a folder (mount your projects with -v <host-folder>:%s)\n", abs, abs)
			continue
		}
		out = append(out, abs)
	}
	return out, nil
}

// tokenCmd issues a new admin token for a running (or the next) `sld serve`.
// Inside the SLD container: docker exec <container> sld token (or: make token).
// The sign-in page shows the exact command for the running container.
var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print a new admin token and a sign-in link for the dashboard",
	Long: `Issues a new admin token and prints it with a link that signs you in.
The running dashboard accepts it immediately; the old token stops working.
Browsers that are already signed in stay signed in.

Run it where SLD keeps its data: inside the SLD container
  docker exec sld sld token            (docker run --name sld setup)
  docker exec sld-app-1 sld token      (compose setup; or: make token)
The dashboard's sign-in page shows the exact command for your container.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := server.IssueToken(hostenv.DefaultDataDir())
		if err != nil {
			return err
		}
		base := strings.TrimRight(os.Getenv("SLD_DASHBOARD_URL"), "/")
		if base == "" {
			base = "http://localhost:2025"
		}
		fmt.Printf("Admin token: %s\n\nSign in: %s/#token=%s\n", token, base, token)
		return nil
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveListen, "listen", "", "Address to listen on (default :2025, or SLD_LISTEN)")
	serveCmd.Flags().BoolVar(&serveResetToken, "reset-token", false, "Issue a new admin token, invalidating the old one")
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(tokenCmd)
}
