package docker

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/supreme-majesty/supreme-local-dev/internal/reconcile"
)

// EngineClient defines the operations required by SLD to manage Docker resources.
type EngineClient interface {
	Ping(ctx context.Context) error
	EnsureNetwork(ctx context.Context, name string, labels map[string]string) (string, error)
	RemoveNetwork(ctx context.Context, nameOrID string) error
	EnsureVolume(ctx context.Context, name string, labels map[string]string) (string, error)
	RemoveVolume(ctx context.Context, name string, force bool) error
	ListProjectVolumes(ctx context.Context, projectID string) ([]string, error)
	ListProjectContainers(ctx context.Context, projectID string) ([]*reconcile.ActualContainer, error)
	ListAllContainers(ctx context.Context) ([]*reconcile.ActualContainer, error)
	CreateContainer(ctx context.Context, desired *reconcile.DesiredContainer) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error
	RemoveContainer(ctx context.Context, containerID string, force bool) error
	ConnectNetwork(ctx context.Context, networkID, containerID string) error
	InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	PullImage(ctx context.Context, imageName string) error
	Close() error
}

// DockerEngine implements EngineClient using the official Docker Go SDK.
type DockerEngine struct {
	cli *client.Client
}

// NewDockerEngine initializes a Docker client negotiating API version from environment.
func NewDockerEngine() (*DockerEngine, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &DockerEngine{cli: cli}, nil
}

// Ping checks whether the Docker Engine daemon is reachable.
func (e *DockerEngine) Ping(ctx context.Context) error {
	_, err := e.cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker daemon is unreachable: %w", err)
	}
	return nil
}

// EnsureNetwork verifies the existence of a network and creates it if missing.
func (e *DockerEngine) EnsureNetwork(ctx context.Context, name string, labels map[string]string) (string, error) {
	f := filters.NewArgs(filters.Arg("name", name))
	existing, err := e.cli.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return "", fmt.Errorf("failed to list networks: %w", err)
	}

	for _, n := range existing {
		if n.Name == name {
			return n.ID, nil
		}
	}

	resp, err := e.cli.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		Labels: labels,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create network %s: %w", name, err)
	}
	return resp.ID, nil
}

// RemoveNetwork deletes a Docker network by exact name or ID if it exists.
func (e *DockerEngine) RemoveNetwork(ctx context.Context, nameOrID string) error {
	f := filters.NewArgs(filters.Arg("name", nameOrID))
	existing, err := e.cli.NetworkList(ctx, network.ListOptions{Filters: f})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	targetID := ""
	for _, n := range existing {
		if n.Name == nameOrID || n.ID == nameOrID {
			targetID = n.ID
			break
		}
	}
	if targetID == "" {
		return nil // Already absent
	}

	if err := e.cli.NetworkRemove(ctx, targetID); err != nil {
		return fmt.Errorf("failed to remove network %s: %w", nameOrID, err)
	}
	return nil
}

// EnsureVolume verifies the existence of a volume and creates it if missing.
func (e *DockerEngine) EnsureVolume(ctx context.Context, name string, labels map[string]string) (string, error) {
	v, err := e.cli.VolumeInspect(ctx, name)
	if err == nil && v.Name == name {
		return v.Name, nil
	}

	resp, err := e.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: labels,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create volume %s: %w", name, err)
	}
	return resp.Name, nil
}

// RemoveVolume deletes a Docker volume by name.
func (e *DockerEngine) RemoveVolume(ctx context.Context, name string, force bool) error {
	if err := e.cli.VolumeRemove(ctx, name, force); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to remove volume %s: %w", name, err)
	}
	return nil
}

// ListProjectVolumes returns all named volumes labeled with the given project ID.
func (e *DockerEngine) ListProjectVolumes(ctx context.Context, projectID string) ([]string, error) {
	f := filters.NewArgs(
		filters.Arg("label", fmt.Sprintf("%s=true", LabelSLD)),
		filters.Arg("label", fmt.Sprintf("%s=%s", LabelProject, projectID)),
	)
	resp, err := e.cli.VolumeList(ctx, volume.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("failed to list project volumes: %w", err)
	}
	names := make([]string, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		names = append(names, v.Name)
	}
	return names, nil
}

// ListProjectContainers returns all containers labeled with a specific project ID.
func (e *DockerEngine) ListProjectContainers(ctx context.Context, projectID string) ([]*reconcile.ActualContainer, error) {
	f := filters.NewArgs(
		filters.Arg("label", fmt.Sprintf("%s=true", LabelSLD)),
		filters.Arg("label", fmt.Sprintf("%s=%s", LabelProject, projectID)),
	)
	return e.queryContainers(ctx, f)
}

// ListAllContainers returns all containers managed by SLD.
func (e *DockerEngine) ListAllContainers(ctx context.Context) ([]*reconcile.ActualContainer, error) {
	f := filters.NewArgs(
		filters.Arg("label", fmt.Sprintf("%s=true", LabelSLD)),
	)
	return e.queryContainers(ctx, f)
}

func (e *DockerEngine) queryContainers(ctx context.Context, f filters.Args) ([]*reconcile.ActualContainer, error) {
	containers, err := e.cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]*reconcile.ActualContainer, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		result = append(result, &reconcile.ActualContainer{
			ID:         c.ID,
			Name:       name,
			Service:    c.Labels[LabelService],
			Image:      c.Image,
			ConfigHash: c.Labels[LabelConfigHash],
			State:      c.State,
			Labels:     c.Labels,
		})
	}
	return result, nil
}

// EnsureImage checks if image is present locally, pulling it if absent.
func (e *DockerEngine) EnsureImage(ctx context.Context, imgName string) error {
	f := filters.NewArgs(filters.Arg("reference", imgName))
	images, err := e.cli.ImageList(ctx, image.ListOptions{Filters: f})
	if err == nil && len(images) > 0 {
		return nil
	}

	reader, err := e.cli.ImagePull(ctx, imgName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imgName, err)
	}
	defer reader.Close()

	// Drain reader to await completion
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// CreateContainer provisions a new container according to the desired specification.
func (e *DockerEngine) CreateContainer(ctx context.Context, desired *reconcile.DesiredContainer) (string, error) {
	// 1. Ensure image is available
	if err := e.EnsureImage(ctx, desired.Image); err != nil {
		return "", err
	}

	// 2. Build Environment
	envList := make([]string, 0, len(desired.Environment))
	for k, v := range desired.Environment {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// 3. Port Bindings
	exposedPorts, portBindings, err := nat.ParsePortSpecs(desired.Ports)
	if err != nil {
		return "", fmt.Errorf("invalid port specifications: %w", err)
	}

	containerConfig := &container.Config{
		Image:        desired.Image,
		Cmd:          desired.Cmd,
		WorkingDir:   desired.WorkingDir,
		Env:          envList,
		Labels:       desired.Labels,
		ExposedPorts: exposedPorts,
	}
	if desired.Healthcheck != nil && len(desired.Healthcheck.Test) > 0 {
		containerConfig.Healthcheck = &container.HealthConfig{
			Test:        desired.Healthcheck.Test,
			Interval:    desired.Healthcheck.Interval,
			Timeout:     desired.Healthcheck.Timeout,
			StartPeriod: desired.Healthcheck.StartPeriod,
			Retries:     desired.Healthcheck.Retries,
		}
	}

	hostConfig := &container.HostConfig{
		Binds:        desired.Volumes,
		PortBindings: portBindings,
		ExtraHosts:   desired.ExtraHosts,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	// 4. Primary Network Configuration
	var netConfig *network.NetworkingConfig
	if len(desired.Networks) > 0 {
		netConfig = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				desired.Networks[0]: {},
			},
		}
	}

	resp, err := e.cli.ContainerCreate(ctx, containerConfig, hostConfig, netConfig, nil, desired.Name)
	if err != nil {
		return "", fmt.Errorf("failed to create container %s: %w", desired.Name, err)
	}

	// 5. Connect secondary networks (e.g. sld-global on Nginx)
	if len(desired.Networks) > 1 {
		for _, netName := range desired.Networks[1:] {
			if err := e.ConnectNetwork(ctx, netName, resp.ID); err != nil {
				return resp.ID, fmt.Errorf("failed to connect secondary network %s: %w", netName, err)
			}
		}
	}

	return resp.ID, nil
}

// StartContainer starts an existing container.
func (e *DockerEngine) StartContainer(ctx context.Context, containerID string) error {
	if err := e.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}
	return nil
}

// StopContainer stops a container with a graceful timeout.
func (e *DockerEngine) StopContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	stopOptions := container.StopOptions{
		Timeout: &timeoutSeconds,
	}
	if err := e.cli.ContainerStop(ctx, containerID, stopOptions); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

// RemoveContainer deletes a container.
func (e *DockerEngine) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	removeOptions := container.RemoveOptions{
		Force: force,
	}
	if err := e.cli.ContainerRemove(ctx, containerID, removeOptions); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}
	return nil
}

// ConnectNetwork attaches a container to a network.
func (e *DockerEngine) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	if err := e.cli.NetworkConnect(ctx, networkID, containerID, nil); err != nil {
		// Ignore if already connected
		if strings.Contains(err.Error(), "already exists in network") {
			return nil
		}
		return fmt.Errorf("failed to connect network %s to container %s: %w", networkID, containerID, err)
	}
	return nil
}

// InspectContainer returns full diagnostic metadata for a container.
func (e *DockerEngine) InspectContainer(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return e.cli.ContainerInspect(ctx, containerID)
}

// ContainerLogs retrieves the logs stream for a container.
func (e *DockerEngine) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return e.cli.ContainerLogs(ctx, containerID, options)
}

// PullImage pulls the specified Docker image from the registry.
func (e *DockerEngine) PullImage(ctx context.Context, imageName string) error {
	reader, err := e.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// Close closes the underlying Docker client connection.
func (e *DockerEngine) Close() error {
	return e.cli.Close()
}
