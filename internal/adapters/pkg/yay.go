package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type YayAdapter struct {
	core.BaseResource
	State string
}

func init() {
	core.RegisterResource("yay", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewYayAdapter(name, params), nil
	})
}

func NewYayAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &YayAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *YayAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for yay")
	}
	return nil
}

func (r *YayAdapter) Check(ctx *core.SystemContext) (bool, error) {
	installed := isInstalled(ctx, "yay", "-Qi", r.Name)
	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *YayAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] yay %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"-Rns", "--noconfirm", r.Name}
	} else {
		args = []string{"-S", "--noconfirm", "--needed", r.Name}
	}

	out, err := runCommand(ctx, "yay", args...)
	if err != nil {
		return core.Failure(err, "Yay failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Yay processed %s", r.Name)), nil
}
