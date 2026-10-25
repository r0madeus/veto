package service

import (
	"fmt"
	"github.com/r0madeus/veto/internal/utils"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

type OpenRCManager struct{}

func NewOpenRCManager() *OpenRCManager {
	return &OpenRCManager{}
}

func (s *OpenRCManager) Name() string {
	return "openrc"
}

func (s *OpenRCManager) IsEnabled(ctx *core.SystemContext, service string) (bool, error) {
	// rc-update show default | grep service
	// This is a bit loose, but standard for OpenRC
	out, _ := ctx.Transport.Execute(ctx.Context, "rc-update show default")
	return strings.Contains(string(out), service), nil
}

func (s *OpenRCManager) IsActive(ctx *core.SystemContext, service string) (bool, error) {
	_, err := ctx.Transport.Execute(ctx.Context, "rc-service "+utils.ShellQuote(service)+" status")
	// rc-service returns 0 if started, non-zero if stopped
	return err == nil, nil
}

func (s *OpenRCManager) Enable(ctx *core.SystemContext, service string) error {
	return s.runUpdate(ctx, "add", service, "default")
}

func (s *OpenRCManager) Disable(ctx *core.SystemContext, service string) error {
	return s.runUpdate(ctx, "del", service, "default")
}

func (s *OpenRCManager) Start(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "start")
}

func (s *OpenRCManager) Stop(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "stop")
}

func (s *OpenRCManager) Restart(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "restart")
}

func (s *OpenRCManager) Reload(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "reload")
}

func (s *OpenRCManager) runService(ctx *core.SystemContext, service string, action string) error {
	fullCmd := "rc-service " + utils.ShellQuote(service) + " " + utils.ShellQuote(action)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return fmt.Errorf("rc-service %s %s failed: %s: %w", service, action, out, err)
	}
	return nil
}

func (s *OpenRCManager) runUpdate(ctx *core.SystemContext, action, service, runlevel string) error {
	fullCmd := "rc-update " + utils.ShellQuote(action) + " " + utils.ShellQuote(service) + " " + utils.ShellQuote(runlevel)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return fmt.Errorf("rc-update %s %s %s failed: %s: %w", action, service, runlevel, out, err)
	}
	return nil
}

func (s *OpenRCManager) ListEnabled(ctx *core.SystemContext) ([]string, error) {
	return nil, fmt.Errorf("list enabled services is not supported for OpenRC yet")
}
