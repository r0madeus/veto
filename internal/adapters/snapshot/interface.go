package snapshot

import "github.com/r0madeus/veto/internal/core"

// Provider defines the interface for snapshot tools
type Provider interface {
	Name() string
	IsAvailable(ctx *core.SystemContext) bool
	// CreateSnapshot creates a single snapshot with description
	CreateSnapshot(ctx *core.SystemContext, description string) error
	// CreatePreSnapshot starts a transactional snapshot (returns transaction ID/Handle)
	CreatePreSnapshot(ctx *core.SystemContext, description string) (string, error)
	// CreatePostSnapshot completes a transactional snapshot using the ID from Pre
	CreatePostSnapshot(ctx *core.SystemContext, id string, description string) error
}
