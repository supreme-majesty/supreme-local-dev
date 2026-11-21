package server

// Writing project files SLD's own user may not write. SLD runs as a non-root
// user, but project folders can hold files a root process created (a
// container running as root, an older SLD that ran as root). Such a write is
// retried in a short-lived helper container running as root, with the
// project mounted the way the Docker daemon sees it.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/supreme-majesty/supreme-local-dev/internal/docker"
	"github.com/supreme-majesty/supreme-local-dev/internal/hostenv"
	"github.com/supreme-majesty/supreme-local-dev/internal/orchestrate"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// writeProjectFile writes data to path inside the project at root, falling
// back to a root helper container when SLD's user is not allowed to.
func (s *Server) writeProjectFile(ctx context.Context, root, path string, data []byte) error {
	err := writeProjectFile(root, path, data)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	if rootErr := s.writeAsRoot(ctx, root, path, data); rootErr != nil {
		return fmt.Errorf("%w (SLD's user may not write it, and writing it as root failed: %v)", err, rootErr)
	}
	return nil
}

// writeAsRoot writes the file as root in a helper container. The file keeps
// its owner and mode; a new file gets the project folder's owner. Inside
// .sld/ (SLD's own files) the folder is given to SLD's user, so later writes
// no longer need the helper.
func (s *Server) writeAsRoot(ctx context.Context, root, path string, data []byte) error {
	executor, ok := s.opts.Engine.(docker.Executor)
	if !ok {
		return errors.New("docker engine cannot run helper containers")
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside the project", path)
	}
	mount, mounted, err := s.opts.Env.MirrorMount(root, false)
	if err != nil {
		return err
	}
	target := filepath.ToSlash(filepath.Join(mounted, rel))

	owner := ""
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		if uid, gid, ok := hostenv.OwnerOf(root); ok {
			owner = uid + ":" + gid
		}
	}
	reclaim := ""
	if parts := strings.Split(filepath.ToSlash(rel), "/"); len(parts) > 1 && parts[0] == ".sld" {
		reclaim = filepath.ToSlash(filepath.Join(mounted, ".sld"))
	}

	// $1 target file, $2 owner for a new file (or ""), $3 .sld folder to give
	// to SLD's user (or ""), $4 SLD's uid:gid.
	script := `set -e
mkdir -p "$(dirname "$1")"
cat > "$1"
if [ -n "$2" ]; then chown "$2" "$1"; fi
if [ -n "$3" ]; then chown -R "$4" "$3"; chmod -R u+rwX "$3"; fi`
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var out bytes.Buffer
	code, err := executor.RunEphemeral(ctx, &reconcile.DesiredContainer{
		Image:  orchestrate.ComposeImage, // already present: SLD runs compose with it
		Mounts: []reconcile.Mount{mount},
	}, docker.ExecOptions{
		Cmd:    []string{"sh", "-c", script, "sh", target, owner, reclaim, fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())},
		User:   "0:0",
		Stdin:  bytes.NewReader(data),
		Stdout: &out,
		Stderr: &out,
	})
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("helper exited with %d: %s", code, strings.TrimSpace(out.String()))
	}
	return nil
}
