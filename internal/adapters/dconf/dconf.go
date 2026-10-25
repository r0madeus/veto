package dconf

import (
	"fmt"
	"github.com/r0madeus/veto/internal/utils"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("dconf", NewDconfAdapter)
}

// DconfAdapter implements resource.Resource for managing dconf settings.
type DconfAdapter struct {
	Name   string // Key (e.g. /org/gnome/desktop/interface/color-scheme)
	State  string // present (default) | reset
	Params map[string]interface{}
}

// NewDconfAdapter creates a new dconf adapter.
func NewDconfAdapter(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	state := "present"
	if s, ok := params["state"].(string); ok && s != "" {
		state = s
	}
	return &DconfAdapter{
		Name:   name,
		State:  state,
		Params: params,
	}, nil
}

func (a *DconfAdapter) GetName() string { return a.Name }
func (a *DconfAdapter) GetType() string { return "dconf" }

func (a *DconfAdapter) Validate(ctx *core.SystemContext) error {
	if a.Name == "" {
		return fmt.Errorf("dconf key path is required")
	}
	if a.State == "present" {
		if _, ok := a.Params["value"]; !ok {
			return fmt.Errorf("value param is required for state=present")
		}
	}
	return nil
}

func (a *DconfAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// 1. Check if dconf binary exists
	if _, err := ctx.Transport.Execute(ctx.Context, "which dconf"); err != nil {
		return false, fmt.Errorf("dconf tool not found")
	}

	// 2. Read current value
	output, err := ctx.Transport.Execute(ctx.Context, fmt.Sprintf("dconf read %s", utils.ShellQuote(a.Name)))
	if err != nil {
		// Treat error as unset/failure to read -> return false if we want to set it?
		// Or return error?
		// If dconf read fails (e.g. key invalid), applying might also fail.
		return false, fmt.Errorf("failed to read dconf key: %w", err)
	}

	currentValue := strings.TrimSpace(output)

	if a.State == "reset" {
		return currentValue != "", nil
	}

	// state == present
	targetValue := fmt.Sprintf("%v", a.Params["value"])
	return currentValue != targetValue, nil
}

func (a *DconfAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	if a.State == "reset" {
		cmd := fmt.Sprintf("dconf reset %s", utils.ShellQuote(a.Name))
		if ctx.DryRun {
			return core.SuccessChange(fmt.Sprintf("[DryRun] Would run: %s", cmd)), nil
		}
		if _, err := ctx.Transport.Execute(ctx.Context, cmd); err != nil {
			return core.Failure(err, "Failed to reset dconf key"), err
		}
		return core.SuccessChange("Reset to default"), nil
	}

	// State = present
	val := fmt.Sprintf("%v", a.Params["value"])
	cmd := fmt.Sprintf("dconf write %s %s", utils.ShellQuote(a.Name), utils.ShellQuote(val))

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Would run: %s", cmd)), nil
	}

	if _, err := ctx.Transport.Execute(ctx.Context, cmd); err != nil {
		return core.Failure(err, fmt.Sprintf("Failed to write dconf: %s", cmd)), err
	}
	return core.SuccessChange(fmt.Sprintf("Set to %s", val)), nil
}
