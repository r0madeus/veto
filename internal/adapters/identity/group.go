package identity

import (
	"fmt"
	"strconv"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/utils"
)

func init() {
	core.RegisterResource("group", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewGroupAdapter(name, params), nil
	})
}

type GroupAdapter struct {
	core.BaseResource
	Gid             int
	System          bool
	State           string
	ActionPerformed string
}

func NewGroupAdapter(name string, params map[string]interface{}) core.Resource {
	gid := -1
	if g, ok := params["gid"].(int); ok {
		gid = g
	} else if gStr, ok := params["gid"].(string); ok {
		gid, _ = strconv.Atoi(gStr)
	}

	system := false
	if s, ok := params["system"].(bool); ok {
		system = s
	}

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	return &GroupAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "group"},
		Gid:          gid,
		System:       system,
		State:        state,
	}
}

func (r *GroupAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("group name is required")
	}
	if !utils.IsValidName(r.Name) {
		return fmt.Errorf("invalid group name '%s': must match regex ^[a-z_][a-z0-9_-]*$", r.Name)
	}
	if !utils.IsOneOf(r.State, "present", "absent") {
		return fmt.Errorf("invalid state '%s': must be one of [present, absent]", r.State)
	}
	return nil
}

func (r *GroupAdapter) Check(ctx *core.SystemContext) (bool, error) {
	provider := GetIdentityProvider(ctx)
	return provider.CheckGroup(ctx, r)
}

func (r *GroupAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Group %s is correct", r.Name)), nil
	}

	provider := GetIdentityProvider(ctx)
	return provider.ApplyGroup(ctx, r)
}

func (r *GroupAdapter) Revert(ctx *core.SystemContext) error {
	provider := GetIdentityProvider(ctx)
	return provider.RevertGroup(ctx, r)
}
