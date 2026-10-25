package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type FlatpakAdapter struct {
	core.BaseResource
	State           string
	ActionPerformed string
}

func init() {
	core.RegisterResource("flatpak", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewFlatpakAdapter(name, params), nil
	})
}

func NewFlatpakAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &FlatpakAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *FlatpakAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for flatpak")
	}
	return nil
}

func (r *FlatpakAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// flatpak info <package>
	installed := isInstalled(ctx, "flatpak", "info", r.Name)

	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *FlatpakAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Flatpak %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] flatpak %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"uninstall", "-y", r.Name}
		r.ActionPerformed = "removed"
	} else {
		// Flatpak genelde non-interactive kurulum için -y ister
		args = []string{"install", "-y", r.Name}
		r.ActionPerformed = "installed"
	}

	out, err := runCommand(ctx, "flatpak", args...)
	if err != nil {
		r.ActionPerformed = ""
		return core.Failure(err, "Flatpak failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Flatpak processed %s", r.Name)), nil
}

func (r *FlatpakAdapter) Revert(ctx *core.SystemContext) error {
	if r.ActionPerformed == "installed" {
		_, err := runCommand(ctx, "flatpak", "uninstall", "-y", r.Name)
		return err
	}
	return nil
}
