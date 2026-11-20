package runtime

import (
	"os"
	"path/filepath"

	"github.com/supreme-majesty/supreme-local-dev/internal/config"
)

// ComposeRoute is one compose service that SLD serves on a domain.
type ComposeRoute struct {
	App        string
	Service    string
	Port       int
	RouterName string
	Hosts      []string
	TLS        bool
}

// ComposeStack is a project's own docker compose stack. Several apps can
// route to different services of the same stack.
type ComposeStack struct {
	Dir       string   // local directory the compose files live in
	Files     []string // compose files relative to Dir
	KeepPorts bool
	Routes    []ComposeRoute
}

// composeDriver plans an app served by the project's own compose stack.
type composeDriver struct{}

func (composeDriver) Plan(ctx *Context, app *config.App) (*AppPlan, error) {
	dir := AppRoot(ctx, app)
	files := app.ComposeFiles
	if len(files) == 0 {
		files = DefaultComposeFiles(dir)
		if len(files) == 0 {
			return nil, errNoComposeFile(dir)
		}
	}
	return &AppPlan{Compose: &ComposeStack{
		Dir:       dir,
		Files:     files,
		KeepPorts: app.KeepPorts,
		Routes: []ComposeRoute{{
			App:        app.Name,
			Service:    app.Service,
			Port:       app.Port,
			RouterName: RouterName(ctx, app),
			Hosts:      AppHosts(ctx.Config, app),
			TLS:        ctx.Config.Domain.TLS,
		}},
	}}, nil
}

// composeFileNames are the files `docker compose` loads by default, in order.
var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// DefaultComposeFiles mirrors docker compose's default lookup: the first
// existing compose file plus its matching override file.
func DefaultComposeFiles(dir string) []string {
	for _, name := range composeFileNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			continue
		}
		files := []string{name}
		ext := filepath.Ext(name)
		base := name[:len(name)-len(ext)]
		for _, e := range []string{".yaml", ".yml"} {
			override := base + ".override" + e
			if _, err := os.Stat(filepath.Join(dir, override)); err == nil {
				files = append(files, override)
				break
			}
		}
		return files
	}
	return nil
}

type composeError string

func (e composeError) Error() string { return string(e) }

func errNoComposeFile(dir string) error {
	return composeError("no compose file (compose.yaml, docker-compose.yml, ...) in " + dir + "; set compose_files")
}
