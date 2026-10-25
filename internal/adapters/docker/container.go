package docker

import (
	"fmt"
	"time"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/utils"
)

func init() {
	core.RegisterResource("docker_container", NewDockerAdapter)
	core.RegisterResource("podman_container", NewPodmanAdapter)
}

type ContainerAdapter struct {
	Name    string
	State   string // running, stopped, absent
	Runtime ContainerRuntime
	Params  map[string]interface{}
}

func NewDockerAdapter(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	return newContainerAdapter(name, params, NewDockerRuntime(ctx))
}

func NewPodmanAdapter(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	return newContainerAdapter(name, params, NewPodmanRuntime(ctx))
}

func newContainerAdapter(name string, params map[string]interface{}, runtime ContainerRuntime) (core.Resource, error) {
	desiredState, _ := params["state"].(string)
	if desiredState == "" {
		desiredState = "running"
	}

	return &ContainerAdapter{
		Name:    name,
		State:   desiredState,
		Runtime: runtime,
		Params:  params,
	}, nil
}

func (a *ContainerAdapter) GetName() string { return a.Name }
func (a *ContainerAdapter) GetType() string { return a.Runtime.Name() + "_container" }

func (a *ContainerAdapter) Validate(ctx *core.SystemContext) error {
	if a.Params["image"] == "" && a.State != "absent" {
		return fmt.Errorf("image is required for container %s", a.Name)
	}
	if !utils.IsOneOf(a.State, "running", "stopped", "absent") {
		return fmt.Errorf("invalid state '%s': must be one of [running, stopped, absent]", a.State)
	}
	return nil
}

func (a *ContainerAdapter) Check(ctx *core.SystemContext) (bool, error) {
	state, err := a.Runtime.Inspect(ctx.Context, a.Name)
	if err != nil {
		return false, err
	}

	exists := state != nil

	if a.State == "absent" {
		return exists, nil // If exists, needs removal
	}

	if !exists {
		return true, nil // Needs creation
	}

	// Check Running State
	if a.State == "running" && !state.Running {
		return true, nil // Stopped but should be running
	}
	if a.State == "stopped" && state.Running {
		return true, nil // Running but should be stopped
	}

	// Check Image Drift
	desiredImage, _ := a.Params["image"].(string)
	// Note: state.ImageName from Inspect might be full SHA or tag-specific.
	// ContainerConfig.Image from inspect is usually the tag name if available.
	// For simplicity, we compare what inspect gave us (Config.Image).
	if desiredImage != "" && state.ImageName != desiredImage {
		return true, nil
	}

	return false, nil
}

func (a *ContainerAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	// Re-inspect to be sure
	state, err := a.Runtime.Inspect(ctx.Context, a.Name)
	if err != nil {
		return core.Failure(err, "Failed to inspect container"), err
	}
	exists := state != nil

	if a.State == "absent" {
		if exists {
			if err := a.Runtime.Remove(ctx.Context, a.Name, true); err != nil {
				return core.Failure(err, "Failed to remove container"), err
			}
			return core.SuccessChange("Container removed"), nil
		}
		return core.SuccessNoChange("Container already absent"), nil
	}

	desiredImage, _ := a.Params["image"].(string)

	if exists {
		// Drift check
		imageChanged := desiredImage != "" && state.ImageName != desiredImage

		if imageChanged {
			if err := a.Runtime.Remove(ctx.Context, a.Name, true); err != nil {
				return core.Failure(err, "Failed to remove component for recreation"), err
			}
			// Fall through to create the container with the new image.
		} else {
			// State correction without recreation
			if a.State == "running" && !state.Running {
				if err := a.Runtime.Start(ctx.Context, a.Name); err != nil {
					return core.Failure(err, "Failed to start container"), err
				}
				return core.SuccessChange("Container started"), nil
			}
			if a.State == "stopped" && state.Running {
				if err := a.Runtime.Stop(ctx.Context, a.Name, 10*time.Second); err != nil {
					return core.Failure(err, "Failed to stop container"), err
				}
				return core.SuccessChange("Container stopped"), nil
			}
			return core.SuccessNoChange("Container up to date"), nil
		}
	}

	// Create / Recreate
	config := a.parseConfig()
	if err := a.Runtime.Run(ctx.Context, a.Name, config); err != nil {
		return core.Failure(err, "Failed to run container"), err
	}

	if a.State == "stopped" {
		// Run created it running (usually), so stop it if desired state is stopped
		// Alternatively 'create' vs 'run'. But 'run -d' is standard.
		// If we use 'create' + 'start', we need 2 steps.
		// For now, Run then Stop is simple CLI wrapper approach.
		if err := a.Runtime.Stop(ctx.Context, a.Name, 5*time.Second); err != nil {
			return core.Failure(err, "Failed to stop newly created container"), err
		}
		return core.SuccessChange("Container created (stopped)"), nil
	}

	return core.SuccessChange("Container created/recreated and started"), nil
}

func (a *ContainerAdapter) parseConfig() *ContainerConfig {
	config := &ContainerConfig{
		Image: a.Params["image"].(string),
	}

	// Parse Ports
	if ports, ok := a.Params["ports"].([]interface{}); ok {
		for _, p := range ports {
			config.Ports = append(config.Ports, fmt.Sprintf("%v", p))
		}
	}

	// Parse Volumes
	if vols, ok := a.Params["volumes"].([]interface{}); ok {
		for _, v := range vols {
			config.Volumes = append(config.Volumes, fmt.Sprintf("%v", v))
		}
	}

	// Parse Env
	if env, ok := a.Params["env"].(map[string]interface{}); ok {
		config.Env = make(map[string]string)
		for k, v := range env {
			config.Env[k] = fmt.Sprintf("%v", v)
		}
	}

	// Restart
	if r, ok := a.Params["restart"].(string); ok {
		config.Restart = r
	}

	return config
}

// Diff shows what would change.
// Since containers are complex, simple generic Diff might be hard, but we can return text.
func (a *ContainerAdapter) Diff(ctx *core.SystemContext) (string, error) {
	state, err := a.Runtime.Inspect(ctx.Context, a.Name)
	if err != nil {
		return "", err
	}

	if a.State == "absent" {
		if state != nil {
			return fmt.Sprintf("- Container %s (running: %v)", a.Name, state.Running), nil
		}
		return "", nil
	}

	if state == nil {
		return fmt.Sprintf("+ Container %s (image: %v)", a.Name, a.Params["image"]), nil
	}

	diff := ""
	desiredImage, _ := a.Params["image"].(string)
	if desiredImage != "" && state.ImageName != desiredImage {
		diff += fmt.Sprintf("Image: %s -> %s\n", state.ImageName, desiredImage)
	}

	if a.State == "running" && !state.Running {
		diff += "State: stopped -> running\n"
	} else if a.State == "stopped" && state.Running {
		diff += "State: running -> stopped\n"
	}

	return diff, nil
}

// ListInstalled implements core.Lister interface for Prune
func (a *ContainerAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	return a.Runtime.List(ctx.Context)
}
