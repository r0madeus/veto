package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type SnapAdapter struct {
	core.BaseResource
	State string
}

func init() {
	core.RegisterResource("snap", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewSnapAdapter(name, params), nil
	})
}

func NewSnapAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &SnapAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *SnapAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for snap")
	}
	return nil
}

func (r *SnapAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// snap list <package>
	installed := isInstalled(ctx, "snap", "list", r.Name)

	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *SnapAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Snap %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] snap %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"remove", r.Name}
	} else {
		args = []string{"install", r.Name}
	}

	out, err := runCommand(ctx, "snap", args...)
	if err != nil {
		return core.Failure(err, "Snap failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Snap processed %s", r.Name)), nil
}
