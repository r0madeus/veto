package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type BrewAdapter struct {
	core.BaseResource
	State string
}

func init() {
	core.RegisterResource("brew", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewBrewAdapter(name, params), nil
	})
}

func NewBrewAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &BrewAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *BrewAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for brew")
	}
	return nil
}

func (r *BrewAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// brew list <package>
	installed := isInstalled(ctx, "brew", "list", r.Name)

	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *BrewAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] brew %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"uninstall", r.Name}
	} else {
		args = []string{"install", r.Name}
	}

	out, err := runCommand(ctx, "brew", args...)
	if err != nil {
		return core.Failure(err, "Brew failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Brew processed %s", r.Name)), nil
}

func (r *BrewAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	// brew leaves: lists packages that are not dependencies of other packages
	output, err := runCommand(ctx, "brew", "leaves")
	if err != nil {
		return nil, fmt.Errorf("failed to list brew packages: %w", err)
	}
	return splitLines(output), nil
}

func (r *BrewAdapter) RemoveBatch(names []string, ctx *core.SystemContext) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"uninstall"}, names...)
	_, err := runCommand(ctx, "brew", args...)
	return err
}
