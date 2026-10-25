package pkg

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

type ZypperAdapter struct {
	core.BaseResource
	State string
}

func init() {
	core.RegisterResource("zypper", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewZypperAdapter(name, params), nil
	})
}

func NewZypperAdapter(name string, params map[string]interface{}) core.Resource {
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &ZypperAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "package"},
		State:        state,
	}
}

func (r *ZypperAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required for zypper")
	}
	return nil
}

func (r *ZypperAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// rpm -q genelde en hızlı yoldur ama zypper search -i de olur
	// rpm -q <package>
	installed := isInstalled(ctx, "rpm", "-q", r.Name)

	if r.State == "absent" {
		return installed, nil
	}
	return !installed, nil
}

func (r *ZypperAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s already %s", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] zypper %s %s", r.State, r.Name)), nil
	}

	var args []string
	if r.State == "absent" {
		args = []string{"remove", "-y", r.Name}
	} else {
		// --non-interactive = -n
		args = []string{"install", "-n", r.Name}
	}

	out, err := runCommand(ctx, "zypper", args...)
	if err != nil {
		return core.Failure(err, "Zypper failed: "+out), err
	}

	return core.SuccessChange(fmt.Sprintf("Zypper processed %s", r.Name)), nil
}

func (r *ZypperAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	// rpm -qa --qf "%{NAME}\n"
	output, err := runCommand(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}
	return splitLines(output), nil
}

func (r *ZypperAdapter) RemoveBatch(names []string, ctx *core.SystemContext) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"remove", "-y"}, names...)
	_, err := runCommand(ctx, "zypper", args...)
	return err
}
