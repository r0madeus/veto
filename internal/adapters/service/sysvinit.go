package service

import (
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

type SysVinitManager struct {
	enableCmd string // "update-rc.d" or "chkconfig"
}

func NewSysVinitManager() *SysVinitManager {
	mgr := &SysVinitManager{}
	if core.IsCommandAvailable("update-rc.d") {
		mgr.enableCmd = "update-rc.d"
	} else if core.IsCommandAvailable("chkconfig") {
		mgr.enableCmd = "chkconfig"
	}
	return mgr
}

func (s *SysVinitManager) Name() string {
	return "sysvinit"
}

func (s *SysVinitManager) IsEnabled(ctx *core.SystemContext, service string) (bool, error) {
	if s.enableCmd == "chkconfig" {
		out, _ := ctx.Transport.Execute(ctx.Context, "chkconfig --list "+utils.ShellQuote(service))
		return strings.Contains(string(out), ":on"), nil
	}
	// update-rc.d doesn't easily show status, assuming false or skipping
	// ls /etc/rc3.d/S*service
	_, err := ctx.Transport.Execute(ctx.Context, fmt.Sprintf("ls /etc/rc*.d/S*%s 2>/dev/null", utils.ShellQuote(service)))
	return err == nil, nil
}

func (s *SysVinitManager) IsActive(ctx *core.SystemContext, service string) (bool, error) {
	_, err := ctx.Transport.Execute(ctx.Context, "service "+utils.ShellQuote(service)+" status")
	return err == nil, nil
}

func (s *SysVinitManager) Enable(ctx *core.SystemContext, service string) error {
	if s.enableCmd == "chkconfig" {
		return s.run(ctx, "chkconfig", service, "on")
	} else if s.enableCmd == "update-rc.d" {
		return s.run(ctx, "update-rc.d", service, "defaults")
	}
	return fmt.Errorf("no enable command found for sysvinit")
}

func (s *SysVinitManager) Disable(ctx *core.SystemContext, service string) error {
	if s.enableCmd == "chkconfig" {
		return s.run(ctx, "chkconfig", service, "off")
	} else if s.enableCmd == "update-rc.d" {
		return s.run(ctx, "update-rc.d", service, "remove")
	}
	return fmt.Errorf("no disable command found for sysvinit")
}

func (s *SysVinitManager) Start(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "start")
}

func (s *SysVinitManager) Stop(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "stop")
}

func (s *SysVinitManager) Restart(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "restart")
}

func (s *SysVinitManager) Reload(ctx *core.SystemContext, service string) error {
	return s.runService(ctx, service, "reload")
}

func (s *SysVinitManager) runService(ctx *core.SystemContext, service string, action string) error {
	fullCmd := "service " + utils.ShellQuote(service) + " " + utils.ShellQuote(action)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return fmt.Errorf("service %s %s failed: %s: %w", service, action, out, err)
	}
	return nil
}

func (s *SysVinitManager) run(ctx *core.SystemContext, cmdName string, args ...string) error {
	fullCmd := cmdName + " " + utils.ShellJoin(args...)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return fmt.Errorf("%s %s failed: %s: %w", cmdName, strings.Join(args, " "), out, err)
	}
	return nil
}

func (s *SysVinitManager) ListEnabled(ctx *core.SystemContext) ([]string, error) {
	return nil, fmt.Errorf("list enabled services is not supported for SysVinit yet")
}
