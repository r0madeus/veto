package bundle

import (
	"github.com/r0madeus/veto/internal/core"
)

// Adapter implements the resource.Adapter interface for logical bundles
type Adapter struct {
	Name string
}

func init() {
	core.RegisterResource("bundle", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewBundleAdapter(name, params, ctx), nil
	})
}

// NewBundleAdapter creates a new bundle adapter.
func NewBundleAdapter(name string, params map[string]interface{}, ctx *core.SystemContext) core.Resource {
	return &Adapter{
		Name: name,
	}
}

// Plan always returns an "apply" action but with no changes.
func (a *Adapter) Plan(ctx *core.SystemContext) (*core.PlanResult, error) {
	return &core.PlanResult{
		Changes: []core.PlanChange{
			{
				Type:   "bundle",
				Name:   a.Name,
				Action: "noop", // Just logical
			},
		},
	}, nil
}

// Apply does nothing but return success.
// Apply does nothing but return success.
func (a *Adapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	return core.Result{
		Changed: false,
		Message: "Bundle applied (logical)",
	}, nil
}

// Check always returns false because a conceptual bundle is always "present".
// It never "needs action" itself, but its children might.
// However, to make it show up in logs as "checked", we can return false.
func (a *Adapter) Check(ctx *core.SystemContext) (bool, error) {
	return false, nil
}

// Revert does nothing.
func (a *Adapter) Revert(ctx *core.SystemContext) error {
	return nil
}

func (a *Adapter) GetName() string {
	return a.Name
}

func (a *Adapter) GetType() string {
	return "bundle"
}

func (a *Adapter) Validate(ctx *core.SystemContext) error {
	return nil
}
