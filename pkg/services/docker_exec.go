package services

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// Exec runs cmd in a running container through the Docker Engine API (SLD's
// image has no docker CLI) and returns its exit code. It implements
// ContainerExecFunc, so the database manager can run dump and restore tools
// inside a database's own container.
func (d *DockerDiscoveryService) Exec(ctx context.Context, containerName string, cmd, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if d == nil || d.cli == nil {
		return -1, fmt.Errorf("docker is not available")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	created, err := d.cli.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		Env:          env,
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, fmt.Errorf("exec in %s: %w", containerName, err)
	}
	resp, err := d.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return -1, fmt.Errorf("attach to exec in %s: %w", containerName, err)
	}
	defer resp.Close()

	if stdin != nil {
		go func() {
			_, _ = io.Copy(resp.Conn, stdin)
			_ = resp.CloseWrite() // EOF, so "mysql < dump" finishes
		}()
	}

	done := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdout, stderr, resp.Reader)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			return -1, fmt.Errorf("exec stream: %w", err)
		}
	case <-ctx.Done():
		return -1, ctx.Err()
	}

	inspect, err := d.cli.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return -1, err
	}
	return inspect.ExitCode, nil
}
