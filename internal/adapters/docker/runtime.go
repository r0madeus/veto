package docker

import (
	"context"
	"time"

	"github.com/r0madeus/veto/internal/core"
)

// ContainerState represents the current state of a container
type ContainerState struct {
	Running   bool
	Status    string // running, exited, dead, etc.
	ImageID   string
	ImageName string
	ExitCode  int
}

// ContainerConfig holds the desired configuration for a container
type ContainerConfig struct {
	Image   string
	Ports   []string          // -p "80:80"
	Volumes []string          // -v "/host:/container"
	Env     map[string]string // -e KEY=VALUE
	Restart string            // --restart always
}

// DockerInspect subset - Common for both Docker and Podman JSON output
type InspectResult struct {
	State struct {
		Running bool
		Status  string
	}
	Config struct {
		Image string
	}
	Image string // ID
}

// ContainerRuntime abstracts the container engine (docker, podman) operations
type ContainerRuntime interface {
	Name() string

	// Inspect retrieves details about a container
	// Returns nil, nil if container does not exist
	Inspect(ctx context.Context, name string) (*ContainerState, error)

	// Create creates and starts a container (equivalent to 'run -d')
	// If a container with the same name exists, it should be handled by the caller (likely removed first)
	Run(ctx context.Context, name string, config *ContainerConfig) error

	// Stop stops a running container
	Stop(ctx context.Context, name string, timeout time.Duration) error

	// Start starts a stopped container
	Start(ctx context.Context, name string) error

	// Remove removes a container (optionally forcing it)
	Remove(ctx context.Context, name string, force bool) error

	// List returns a list of all container names (used for pruning/inventory)
	List(ctx context.Context) ([]string, error)
}

// Ensure interface compliance helper
var _ ContainerRuntime = (*DockerRuntime)(nil)
var _ ContainerRuntime = (*PodmanRuntime)(nil)

// Factory function type
type RuntimeFactory func(ctx *core.SystemContext) ContainerRuntime
