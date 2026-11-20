package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/archive"
	"github.com/moby/patternmatcher/ignorefile"
)

// BuildOptions describes an image build from a local directory.
type BuildOptions struct {
	ContextDir string            // absolute local path
	Dockerfile string            // relative to ContextDir
	Target     string            // optional build stage
	BuildArgs  map[string]string // --build-arg values
	Tag        string
	Output     io.Writer // build log (nil = discard)
}

// Builder builds images through the Docker Engine API.
type Builder interface {
	// BuildImage builds and tags an image and returns its ID.
	BuildImage(ctx context.Context, opts BuildOptions) (string, error)
}

var _ Builder = (*DockerEngine)(nil)

// BuildImage implements Builder. The context honours .dockerignore.
func (e *DockerEngine) BuildImage(ctx context.Context, opts BuildOptions) (string, error) {
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}
	if _, err := os.Stat(filepath.Join(opts.ContextDir, filepath.FromSlash(opts.Dockerfile))); err != nil {
		return "", fmt.Errorf("dockerfile not found: %w", err)
	}

	excludes, err := readDockerignore(opts.ContextDir)
	if err != nil {
		return "", err
	}
	buildCtx, err := archive.TarWithOptions(opts.ContextDir, &archive.TarOptions{ExcludePatterns: excludes})
	if err != nil {
		return "", fmt.Errorf("failed to package build context: %w", err)
	}
	defer buildCtx.Close()

	args := make(map[string]*string, len(opts.BuildArgs))
	for k, v := range opts.BuildArgs {
		args[k] = &v
	}

	resp, err := e.cli.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Dockerfile:  opts.Dockerfile,
		Tags:        []string{opts.Tag},
		Target:      opts.Target,
		BuildArgs:   args,
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return "", fmt.Errorf("image build failed: %w", err)
	}
	defer resp.Body.Close()

	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	imageID, err := readBuildStream(resp.Body, out)
	if err != nil {
		return "", err
	}
	if imageID == "" {
		inspect, _, err := e.cli.ImageInspectWithRaw(ctx, opts.Tag)
		if err != nil {
			return "", fmt.Errorf("built image %s not found: %w", opts.Tag, err)
		}
		imageID = inspect.ID
	}
	return imageID, nil
}

// readBuildStream relays build output and returns the image ID if reported.
func readBuildStream(r io.Reader, out io.Writer) (string, error) {
	dec := json.NewDecoder(r)
	imageID := ""
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
			Aux    *struct {
				ID string `json:"ID"`
			} `json:"aux"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return imageID, nil
			}
			return "", fmt.Errorf("failed to read build output: %w", err)
		}
		if msg.Error != "" {
			return "", fmt.Errorf("image build failed: %s", strings.TrimSpace(msg.Error))
		}
		if msg.Stream != "" {
			_, _ = io.WriteString(out, msg.Stream)
		}
		if msg.Aux != nil && msg.Aux.ID != "" {
			imageID = msg.Aux.ID
		}
	}
}

func readDockerignore(dir string) ([]string, error) {
	f, err := os.Open(filepath.Join(dir, ".dockerignore"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	patterns, err := ignorefile.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("invalid .dockerignore: %w", err)
	}
	return patterns, nil
}
