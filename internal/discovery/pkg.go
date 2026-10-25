package discovery

import (
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

func discoverPackages(ctx *core.SystemContext) ([]string, error) {
	var allPkgs []string

	// 1. System Package Managers
	if isCommandAvailable(ctx, "paru") {
		p, _ := discoverGeneric(ctx, "paru", "-Qqe")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "yay") {
		p, _ := discoverGeneric(ctx, "yay", "-Qqe")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "pacman") {
		p, _ := discoverGeneric(ctx, "pacman", "-Qqe")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "dnf") {
		p, _ := discoverGeneric(ctx, "dnf", "repoquery", "--userinstalled", "--queryformat", "%{name}")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "yum") {
		p, _ := discoverGeneric(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "zypper") {
		p, _ := discoverGeneric(ctx, "rpm", "-qa", "--qf", "%{NAME}\n")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "apt") {
		p, _ := discoverGeneric(ctx, "apt-mark", "showmanual")
		allPkgs = append(allPkgs, p...)
	} else if isCommandAvailable(ctx, "apk") {
		p, _ := discoverGeneric(ctx, "apk", "info")
		allPkgs = append(allPkgs, p...)
	}

	// 2. Extra Managers (can coexist)
	if isCommandAvailable(ctx, "brew") {
		p, _ := discoverGeneric(ctx, "brew", "leaves")
		allPkgs = append(allPkgs, p...)
	}
	if isCommandAvailable(ctx, "flatpak") {
		p, _ := discoverGeneric(ctx, "flatpak", "list", "--app", "--columns=application")
		allPkgs = append(allPkgs, p...)
	}
	if isCommandAvailable(ctx, "snap") {
		if out, err := ctx.Transport.Execute(ctx.Context, "snap list"); err == nil {
			lines := parseLines([]byte(out))
			if len(lines) > 0 {
				for _, line := range lines[1:] {
					fields := strings.Fields(line)
					if len(fields) > 0 {
						allPkgs = append(allPkgs, fields[0])
					}
				}
			}
		}
	}

	if len(allPkgs) == 0 {
		return nil, fmt.Errorf("no supported package manager found or no packages detected")
	}

	return unique(allPkgs), nil
}

func discoverGeneric(ctx *core.SystemContext, cmd string, args ...string) ([]string, error) {
	fullCmd := cmd + " " + utils.ShellJoin(args...)
	output, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return nil, err
	}
	return parseLines([]byte(output)), nil
}
