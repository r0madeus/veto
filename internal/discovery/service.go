package discovery

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

func discoverServices(ctx *core.SystemContext) ([]string, error) {
	switch ctx.InitSystem {
	case "systemd":
		return discoverSystemd(ctx)
	case "openrc":
		return discoverOpenRC(ctx)
	case "sysvinit":
		return discoverSysVinit(ctx)
	default:
		return nil, fmt.Errorf("unsupported init system: %s", ctx.InitSystem)
	}
}

func discoverSystemd(ctx *core.SystemContext) ([]string, error) {
	// systemctl list-unit-files --state=enabled --type=service --no-legend
	output, err := ctx.Transport.Execute(ctx.Context, "systemctl list-unit-files --state=enabled --type=service --no-legend")
	if err != nil {
		return nil, err
	}

	var services []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 0 {
			svc := fields[0]
			// remove .service suffix
			svc = strings.TrimSuffix(svc, ".service")
			services = append(services, svc)
		}
	}
	return services, nil
}

func discoverOpenRC(ctx *core.SystemContext) ([]string, error) {
	// rc-update show default
	output, err := ctx.Transport.Execute(ctx.Context, "rc-update show default")
	if err != nil {
		return nil, err
	}

	var services []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 0 {
			services = append(services, fields[0])
		}
	}
	return services, nil
}

func discoverSysVinit(ctx *core.SystemContext) ([]string, error) {
	// This is messy across distros.
	// Debian/Ubuntu: service --status-all | grep +
	// But sysvinit is rare now. Returning empty for now.
	return []string{}, nil
}
