package core

// Resource is the interface representing a manageable unit in the system.
// Solves Import Cycle issue by being in the Core package.
type Resource interface {
	Apply(ctx *SystemContext) (Result, error)
	Check(ctx *SystemContext) (bool, error)
	Validate(ctx *SystemContext) error
	GetName() string
	GetType() string
}

// Differ is the interface for resources that can provide a diff of changes.
type Differ interface {
	Diff(ctx *SystemContext) (string, error)
}

// Reverter is implemented by every resource that can undo what Apply did.
// The engine uses it to roll back a failed run.
type Reverter interface {
	Revert(ctx *SystemContext) error
}

// Revertable additionally supports reverting a recorded action by name; it is
// what `veto rollback` needs to undo a past transaction.
type Revertable interface {
	Revert(ctx *SystemContext) error
	RevertAction(action string, ctx *SystemContext) error
}

// Lister is the interface for resources that can enumerate installed instances.
// Required for Prune operations.
type Lister interface {
	ListInstalled(ctx *SystemContext) ([]string, error)
}

// BatchRemover is the interface for resources that support removing multiple items at once.
// Used in Prune operations for performance optimization.
type BatchRemover interface {
	RemoveBatch(names []string, ctx *SystemContext) error
} // <--- Added

// BaseResource holds common fields.
type BaseResource struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

func (b *BaseResource) GetName() string {
	return b.Name
}

func (b *BaseResource) GetType() string {
	return b.Type
}
