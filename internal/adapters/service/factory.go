package service

import (
	"github.com/r0madeus/veto/internal/core"
)

func GetServiceManager(ctx *core.SystemContext) ServiceManager {
	// If context is nil (should not happen in normal flow but for safety) defaults to systemd
	if ctx == nil {
		return NewSystemdManager()
	}

	switch ctx.InitSystem {
	case "systemd":
		return NewSystemdManager()
	case "openrc":
		return NewOpenRCManager()
	case "sysvinit":
		return NewSysVinitManager()
	default:
		// Fallback detection if unknown
		// This might be redundant if detection logic covers everything,
		// but safe to default to systemd or try others.
		return NewSystemdManager()
	}
}
