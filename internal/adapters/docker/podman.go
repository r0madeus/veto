package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/r0madeus/veto/internal/utils"
	"strings"
	"time"

	"github.com/r0madeus/veto/internal/core"
)

type PodmanRuntime struct {
	ctx *core.SystemContext
}

func NewPodmanRuntime(ctx *core.SystemContext) ContainerRuntime {
	return &PodmanRuntime{ctx: ctx}
}

func (r *PodmanRuntime) Name() string {
	return "podman"
}

func (r *PodmanRuntime) runCmd(ctx context.Context, args ...string) (string, error) {
	cmdStr := "podman " + utils.ShellJoin(args...)
	return r.ctx.Transport.Execute(ctx, cmdStr)
}

func (r *PodmanRuntime) Inspect(ctx context.Context, name string) (*ContainerState, error) {
	out, err := r.runCmd(ctx, "inspect", name)
	if err != nil {
		return nil, nil
	}

	var results []InspectResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		return nil, fmt.Errorf("failed to parse podman inspect: %w", err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	container := results[0]
	return &ContainerState{
		Running:   container.State.Running,
		Status:    container.State.Status,
		ImageID:   container.Image,
		ImageName: container.Config.Image,
		ExitCode:  0,
	}, nil
}

func (r *PodmanRuntime) Run(ctx context.Context, name string, config *ContainerConfig) error {
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

func (r *PodmanRuntime) Stop(ctx context.Context, name string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	if seconds == 0 {
		seconds = 10
	}
	_, err := r.runCmd(ctx, "stop", "-t", fmt.Sprintf("%d", seconds), name)
	return err
}

func (r *PodmanRuntime) Start(ctx context.Context, name string) error {
	_, err := r.runCmd(ctx, "start", name)
	return err
}

func (r *PodmanRuntime) Remove(ctx context.Context, name string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, err := r.runCmd(ctx, args...)
	return err
}

func (r *PodmanRuntime) List(ctx context.Context) ([]string, error) {
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
