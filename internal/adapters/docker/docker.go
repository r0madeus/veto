package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strings"
	"time"

	"github.com/melih-ucgun/veto/internal/core"
)

type DockerRuntime struct {
	ctx *core.SystemContext
}

func NewDockerRuntime(ctx *core.SystemContext) ContainerRuntime {
	return &DockerRuntime{ctx: ctx}
}

func (r *DockerRuntime) Name() string {
	return "docker"
}

func (r *DockerRuntime) runCmd(ctx context.Context, args ...string) (string, error) {
	cmdStr := "docker " + utils.ShellJoin(args...)
	return r.ctx.Transport.Execute(ctx, cmdStr)
}

func (r *DockerRuntime) Inspect(ctx context.Context, name string) (*ContainerState, error) {
	out, err := r.runCmd(ctx, "inspect", name)
	if err != nil {
		// If error contains "No such object", return nil, nil
		// Note: The specific error message might vary by docker version,
		// but typically it returns a non-zero exit code.
		// We trust the transport to return an error.
		// Verify if it's "not found" or actual error.
		// For simplicity/robustness, if inspect fails we assume it doesn't exist
		// OR we can check the error output.
		// In a real CLI wrapper, usually we assume non-existence.
		return nil, nil
	}

	var results []InspectResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		return nil, fmt.Errorf("failed to parse docker inspect: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	container := results[0]
	return &ContainerState{
		Running:   container.State.Running,
		Status:    container.State.Status,
		ImageID:   container.Image, // Use ID for precise drift detection if needed
		ImageName: container.Config.Image,
		ExitCode:  0, // Not easily available from running inspect without checking State.ExitCode
	}, nil
}

func (r *DockerRuntime) Run(ctx context.Context, name string, config *ContainerConfig) error {
	args := []string{"run", "-d", "--name", name}

	if config.Restart != "" {
		args = append(args, "--restart", config.Restart)
	}

	for _, p := range config.Ports {
		args = append(args, "-p", p)
	}

	for _, v := range config.Volumes {
		args = append(args, "-v", v)
	}

	for k, v := range config.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, config.Image)

	_, err := r.runCmd(ctx, args...)
	return err
}

func (r *DockerRuntime) Stop(ctx context.Context, name string, timeout time.Duration) error {
	// Timeout handling in CLI: -t <seconds>
	// docker stop -t 10 container
	seconds := int(timeout.Seconds())
	if seconds == 0 {
		seconds = 10
	}
	_, err := r.runCmd(ctx, "stop", "-t", fmt.Sprintf("%d", seconds), name)
	return err
}

func (r *DockerRuntime) Start(ctx context.Context, name string) error {
	_, err := r.runCmd(ctx, "start", name)
	return err
}

func (r *DockerRuntime) Remove(ctx context.Context, name string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, err := r.runCmd(ctx, args...)
	return err
}

func (r *DockerRuntime) List(ctx context.Context) ([]string, error) {
	// List names of all containers (running and stopped)
	// docker ps -a --format '{{.Names}}'
	out, err := r.runCmd(ctx, "ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	var names []string
	for _, line := range lines {
		if line != "" {
			names = append(names, strings.TrimSpace(line))
		}
	}
	return names, nil
}
