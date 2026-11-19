package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// ExecOptions describes a command attached to the caller's stdio.
type ExecOptions struct {
	Cmd        []string
	Env        []string // KEY=value
	WorkingDir string
	User       string
	TTY        bool          // allocate a pseudo-terminal (stdout/stderr are not demultiplexed)
	Stdin      io.Reader     // nil = no stdin
	Stdout     io.Writer     // required
	Stderr     io.Writer     // used when TTY is false; defaults to Stdout
	ConsoleSize *[2]uint     // optional initial terminal size {height, width}
}

// Executor runs commands in containers through the Docker Engine API, so SLD
// does not depend on a host `docker` CLI (which is absent inside the SLD image).
type Executor interface {
	// Exec runs a command in an existing, running container and returns its exit code.
	Exec(ctx context.Context, containerID string, opts ExecOptions) (int, error)
	// RunEphemeral creates a container from desired, attaches stdio, waits for it
	// to exit, removes it, and returns its exit code. desired.Cmd is the command.
	RunEphemeral(ctx context.Context, desired *reconcile.DesiredContainer, opts ExecOptions) (int, error)
}

var _ Executor = (*DockerEngine)(nil)

// Exec implements Executor.
func (e *DockerEngine) Exec(ctx context.Context, containerID string, opts ExecOptions) (int, error) {
	if len(opts.Cmd) == 0 {
		return -1, fmt.Errorf("exec: empty command")
	}
	created, err := e.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          opts.Cmd,
		Env:          opts.Env,
		WorkingDir:   opts.WorkingDir,
		User:         opts.User,
		Tty:          opts.TTY,
		ConsoleSize:  opts.ConsoleSize,
		AttachStdin:  opts.Stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, fmt.Errorf("failed to create exec in %s: %w", containerID, err)
	}

	resp, err := e.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{
		Tty:         opts.TTY,
		ConsoleSize: opts.ConsoleSize,
	})
	if err != nil {
		return -1, fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer resp.Close()

	if err := pumpStreams(ctx, resp, opts); err != nil {
		return -1, err
	}

	inspect, err := e.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return -1, fmt.Errorf("failed to inspect exec result: %w", err)
	}
	return inspect.ExitCode, nil
}

// RunEphemeral implements Executor.
func (e *DockerEngine) RunEphemeral(ctx context.Context, desired *reconcile.DesiredContainer, opts ExecOptions) (int, error) {
	if err := e.EnsureImage(ctx, desired.Image); err != nil {
		return -1, err
	}

	env := make([]string, 0, len(desired.Environment)+len(opts.Env))
	for k, v := range desired.Environment {
		env = append(env, k+"="+v)
	}
	env = append(env, opts.Env...)

	cmd := desired.Cmd
	if len(opts.Cmd) > 0 {
		cmd = opts.Cmd
	}

	cfg := &container.Config{
		Image:        desired.Image,
		Cmd:          cmd,
		Env:          env,
		WorkingDir:   firstNonEmpty(opts.WorkingDir, desired.WorkingDir),
		User:         firstNonEmpty(opts.User, desired.User),
		Labels:       desired.Labels,
		Tty:          opts.TTY,
		OpenStdin:    opts.Stdin != nil,
		StdinOnce:    opts.Stdin != nil,
		AttachStdin:  opts.Stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
	}
	hostCfg := &container.HostConfig{
		Binds:      desired.Volumes,
		Mounts:     toDockerMounts(desired.Mounts),
		ExtraHosts: desired.ExtraHosts,
	}
	if opts.ConsoleSize != nil {
		hostCfg.ConsoleSize = *opts.ConsoleSize
	}
	var netCfg *network.NetworkingConfig
	if len(desired.Networks) > 0 {
		netCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{desired.Networks[0]: {}},
		}
	}

	created, err := e.cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, desired.Name)
	if err != nil {
		return -1, fmt.Errorf("failed to create ephemeral container: %w", err)
	}
	// Always clean up, even if the caller's context was cancelled.
	defer func() {
		_ = e.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	}()

	resp, err := e.cli.ContainerAttach(ctx, created.ID, container.AttachOptions{
		Stream: true,
		Stdin:  opts.Stdin != nil,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return -1, fmt.Errorf("failed to attach to ephemeral container: %w", err)
	}
	defer resp.Close()

	waitCh, errCh := e.cli.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)

	if err := e.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return -1, fmt.Errorf("failed to start ephemeral container: %w", err)
	}

	if err := pumpStreams(ctx, resp, opts); err != nil {
		return -1, err
	}

	select {
	case res := <-waitCh:
		if res.Error != nil {
			return -1, fmt.Errorf("ephemeral container wait error: %s", res.Error.Message)
		}
		return int(res.StatusCode), nil
	case err := <-errCh:
		return -1, fmt.Errorf("failed waiting for ephemeral container: %w", err)
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// pumpStreams copies stdin to the hijacked connection and the connection's
// output to stdout/stderr until the remote side closes its output.
func pumpStreams(ctx context.Context, resp types.HijackedResponse, opts ExecOptions) error {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = stdout
	}

	if opts.Stdin != nil {
		go func() {
			_, _ = io.Copy(resp.Conn, opts.Stdin)
			_ = resp.CloseWrite() // signal EOF so commands reading stdin (mysql < dump) finish
		}()
	}

	outDone := make(chan error, 1)
	go func() {
		var err error
		if opts.TTY {
			_, err = io.Copy(stdout, resp.Reader)
		} else {
			_, err = stdcopy.StdCopy(stdout, stderr, resp.Reader)
		}
		outDone <- err
	}()

	select {
	case err := <-outDone:
		if err != nil && err != io.EOF {
			return fmt.Errorf("stream error: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
