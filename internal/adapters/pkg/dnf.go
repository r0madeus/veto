package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type DnfAdapter struct {
	core.BaseResource
	State           string
	ActionPerformed string
}

func init() {
	core.RegisterResource("dnf", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewDnfAdapter(name, params), nil
	})
}

func NewDnfAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &DnfAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *DnfAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required")
	}
	if r.State != "present" && r.State != "absent" {
		return fmt.Errorf("invalid state '%s', must be 'present' or 'absent'", r.State)
	}
	return nil
}

func (r *DnfAdapter) Check(ctx *core.SystemContext) (bool, error) {
	installed := isInstalled(ctx, "rpm", "-q", r.Name)
	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *DnfAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] dnf %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"remove", "-y", r.Name}
		r.ActionPerformed = "removed"
	} else {
		args = []string{"install", "-y", r.Name}
		r.ActionPerformed = "installed"
	}

	out, err := runCommand(ctx, "dnf", args...)
	if err != nil {
		r.ActionPerformed = ""
		return core.Failure(err, "Dnf failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Dnf processed %s", r.Name)), nil
}

func (r *DnfAdapter) Revert(ctx *core.SystemContext) error {
	if r.ActionPerformed == "installed" {
		_, err := runCommand(ctx, "dnf", "remove", "-y", r.Name)
		return err
	} else if r.ActionPerformed == "removed" {
		_, err := runCommand(ctx, "dnf", "install", "-y", r.Name)
		return err
	}
	return nil
}

func (r *DnfAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	// rpm -qa --qf "%{NAME}\n" lists all installed package names
	output, err := runCommand(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}
	return splitLines(output), nil
}

func (r *DnfAdapter) RemoveBatch(names []string, ctx *core.SystemContext) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, names...)
	_, err := runCommand(ctx, "dnf", args...)
	return err
}
