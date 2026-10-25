package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type ParuAdapter struct {
	core.BaseResource
	State           string
	ActionPerformed string
}

func init() {
	core.RegisterResource("paru", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewParuAdapter(name, params), nil
	})
}

func NewParuAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &ParuAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *ParuAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for paru")
	}
	return nil
}

func (r *ParuAdapter) Check(ctx *core.SystemContext) (bool, error) {
	installed := isInstalled(ctx, "paru", "-Qi", r.Name)
	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *ParuAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] paru %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"-Rns", "--noconfirm", r.Name}
		r.ActionPerformed = "removed"
	} else {
		args = []string{"-S", "--noconfirm", "--needed", r.Name}
		r.ActionPerformed = "installed"
	}

	out, err := runCommand(ctx, "paru", args...)
	if err != nil {
		r.ActionPerformed = ""
		return core.Failure(err, "Paru failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Paru processed %s", r.Name)), nil
}

func (r *ParuAdapter) Revert(ctx *core.SystemContext) error {
	if r.ActionPerformed == "installed" {
		_, err := runCommand(ctx, "paru", "-Rns", "--noconfirm", r.Name)
		return err
	}
	return nil
}
