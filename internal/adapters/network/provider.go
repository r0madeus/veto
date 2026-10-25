package network

import (
	"github.com/r0madeus/veto/internal/core"
)

// FirewallProvider defines the interface for firewall operations
type FirewallProvider interface {
	CheckRule(ctx *core.SystemContext, r *FirewallAdapter) (bool, error)
	ApplyRule(ctx *core.SystemContext, r *FirewallAdapter) (core.Result, error)
	RevertRule(ctx *core.SystemContext, r *FirewallAdapter) error
}
