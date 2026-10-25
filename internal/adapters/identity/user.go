package identity

import (
	"fmt"
	"strings"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/utils"
)

func init() {
	core.RegisterResource("user", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewUserAdapter(name, params), nil
	})
}

type UserAdapter struct {
	core.BaseResource
	Uid             string
	Gid             string
	Groups          []string // Ek gruplar
	Home            string
	Shell           string
	System          bool
	State           string
	ActionPerformed string
}

func NewUserAdapter(name string, params map[string]interface{}) core.Resource {
	uid, _ := params["uid"].(string)
	gid, _ := params["gid"].(string)
	home, _ := params["home"].(string)
	shell, _ := params["shell"].(string)

	system := false
	if s, ok := params["system"].(bool); ok {
		system = s
	}

	var groups []string
	if gList, ok := params["groups"].([]interface{}); ok {
		for _, g := range gList {
			if gStr, ok := g.(string); ok {
				groups = append(groups, gStr)
			}
		}
	} else if gStr, ok := params["groups"].(string); ok {
		// Virgülle ayrılmış string desteği
		groups = strings.Split(gStr, ",")
	}

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	return &UserAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "user"},
		Uid:          uid,
		Gid:          gid,
		Groups:       groups,
		Home:         home,
		Shell:        shell,
		System:       system,
		State:        state,
	}
}

func (r *UserAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("username is required")
	}
	if !utils.IsValidName(r.Name) {
		return fmt.Errorf("invalid username '%s': must match regex ^[a-z_][a-z0-9_-]*$", r.Name)
	}
	if !utils.IsOneOf(r.State, "present", "absent") {
		return fmt.Errorf("invalid state '%s': must be one of [present, absent]", r.State)
	}
	return nil
}

// Check verifies if the user exists and matches the desired state.
func (r *UserAdapter) Check(ctx *core.SystemContext) (bool, error) {
	provider := GetIdentityProvider(ctx)
	return provider.CheckUser(ctx, r)
}

func (r *UserAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("User %s is correct", r.Name)), nil
	}

	provider := GetIdentityProvider(ctx)
	return provider.ApplyUser(ctx, r)
}

func (r *UserAdapter) Revert(ctx *core.SystemContext) error {
	provider := GetIdentityProvider(ctx)
	return provider.RevertUser(ctx, r)
}
