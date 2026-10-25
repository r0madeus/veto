package service

import (
	"github.com/r0madeus/veto/internal/core"
)

// ServiceManager, init sistemleri (systemd, openrc, vb.) için ortak arayüzdür.
type ServiceManager interface {
	Name() string // systemd, openrc

	// State Checks
	IsEnabled(ctx *core.SystemContext, service string) (bool, error)
	IsActive(ctx *core.SystemContext, service string) (bool, error)

	// Actions
	Enable(ctx *core.SystemContext, service string) error
	Disable(ctx *core.SystemContext, service string) error
	Start(ctx *core.SystemContext, service string) error
	Stop(ctx *core.SystemContext, service string) error
	Restart(ctx *core.SystemContext, service string) error
	Reload(ctx *core.SystemContext, service string) error

	// Discovery
	ListEnabled(ctx *core.SystemContext) ([]string, error)
}
