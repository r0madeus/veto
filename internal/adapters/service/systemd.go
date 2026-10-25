package service

import (
	"fmt"
	"github.com/r0madeus/veto/internal/utils"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

type SystemdManager struct{}

func NewSystemdManager() *SystemdManager {
	return &SystemdManager{}
}

func (s *SystemdManager) Name() string {
	return "systemd"
}

func (s *SystemdManager) IsEnabled(ctx *core.SystemContext, service string) (bool, error) {
	out, _ := ctx.Transport.Execute(ctx.Context, "systemctl is-enabled "+utils.ShellQuote(service)) // Error is expected if disabled
	status := strings.TrimSpace(string(out))
	return status == "enabled", nil
}

func (s *SystemdManager) IsActive(ctx *core.SystemContext, service string) (bool, error) {
	out, _ := ctx.Transport.Execute(ctx.Context, "systemctl is-active "+utils.ShellQuote(service)) // Error is expected if inactive
	status := strings.TrimSpace(string(out))
	return status == "active", nil
}

func (s *SystemdManager) Enable(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "enable", "--now", service)
}

func (s *SystemdManager) Disable(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "disable", "--now", service)
}

func (s *SystemdManager) Start(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "start", service)
}

func (s *SystemdManager) Stop(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "stop", service)
}

func (s *SystemdManager) Restart(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "restart", service)
}

func (s *SystemdManager) Reload(ctx *core.SystemContext, service string) error {
	return s.run(ctx, "reload", service)
}

func (s *SystemdManager) run(ctx *core.SystemContext, args ...string) error {
	fullCmd := "systemctl " + utils.ShellJoin(args...)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return fmt.Errorf("systemctl %s failed: %s: %w", strings.Join(args, " "), out, err)
	}
	return nil
}

func (s *SystemdManager) ListEnabled(ctx *core.SystemContext) ([]string, error) {
	// systemctl list-unit-files --state=enabled --type=service --no-legend --no-pager
	fullCmd := "systemctl list-unit-files --state=enabled --type=service --no-legend --no-pager"
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled services: %w", err)
	}

	var services []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			// fields[0] is service name (e.g., "sshd.service")
			services = append(services, fields[0])
		}
	}
	return services, nil
}
