package identity

import (
	"github.com/r0madeus/veto/internal/core"
)

// IdentityProvider defines the interface for OS-specific user/group operations.
type IdentityProvider interface {
	CheckUser(ctx *core.SystemContext, r *UserAdapter) (bool, error)
	ApplyUser(ctx *core.SystemContext, r *UserAdapter) (core.Result, error)
	RevertUser(ctx *core.SystemContext, r *UserAdapter) error

	CheckGroup(ctx *core.SystemContext, r *GroupAdapter) (bool, error)
	ApplyGroup(ctx *core.SystemContext, r *GroupAdapter) (core.Result, error)
	RevertGroup(ctx *core.SystemContext, r *GroupAdapter) error
}
