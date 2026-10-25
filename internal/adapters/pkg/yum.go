package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type YumAdapter struct {
	core.BaseResource
	State string
}

func init() {
	core.RegisterResource("yum", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewYumAdapter(name, params), nil
	})
}

func NewYumAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &YumAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *YumAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for yum")
	}
	return nil
}

func (r *YumAdapter) Check(ctx *core.SystemContext) (bool, error) {
	installed := isInstalled(ctx, "rpm", "-q", r.Name)
	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *YumAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] yum %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"remove", "-y", r.Name}
	} else {
		args = []string{"install", "-y", r.Name}
	}

	out, err := runCommand(ctx, "yum", args...)
	if err != nil {
		return core.Failure(err, "Yum failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Yum processed %s", r.Name)), nil
}

func (r *YumAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	output, err := runCommand(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}
	return splitLines(output), nil
}

func (r *YumAdapter) RemoveBatch(names []string, ctx *core.SystemContext) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, names...)
	_, err := runCommand(ctx, "yum", args...)
	return err
}
