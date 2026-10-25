package pkg

import (
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

func init() {
	core.RegisterResource("package", DetectPackageManager)
	core.RegisterResource("pkg", DetectPackageManager)
}

// DetectPackageManager detects and returns the appropriate package manager adapter.
func DetectPackageManager(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	pkgName, _ := params["name"].(string)
	if pkgName == "" {
		pkgName = name
	}

	switch ctx.Distro {
	case "arch", "cachyos", "manjaro", "endeavouros":
		return NewPacmanAdapter(pkgName, params), nil
	case "ubuntu", "debian", "pop", "mint", "kali":
		return NewAptAdapter(pkgName, params), nil
	case "fedora", "rhel", "centos", "almalinux":
		return NewDnfAdapter(pkgName, params), nil
	case "alpine":
		return NewApkAdapter(pkgName, params), nil
	case "opensuse", "sles":
		return NewZypperAdapter(pkgName, params), nil
	case "darwin":
		return NewBrewAdapter(pkgName, params), nil
	default:
		// Fallback to searching available commands
		if core.IsCommandAvailable("pacman") {
			return NewPacmanAdapter(pkgName, params), nil
		} else if core.IsCommandAvailable("apt-get") {
			return NewAptAdapter(pkgName, params), nil
		} else if core.IsCommandAvailable("dnf") {
			return NewDnfAdapter(pkgName, params), nil
		}

		return nil, fmt.Errorf("automatic package manager detection failed for distro: %s", ctx.Distro)
	}
}

// isInstalled, verilen komutun başarıyla çalışıp çalışmadığını kontrol eder.
// Paket yöneticileri genellikle paket varsa 0, yoksa hata kodu döner.
func isInstalled(ctx *core.SystemContext, checkCmd string, args ...string) bool {
	fullCmd := checkCmd
	if len(args) > 0 {
		fullCmd += " " + utils.ShellJoin(args...)
	}

	_, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	return err == nil
}

// runCommand, bir komutu çalıştırır ve çıktısını/hatasını döner.
func runCommand(ctx *core.SystemContext, name string, args ...string) (string, error) {
	fullCmd := name
	if len(args) > 0 {
		fullCmd += " " + utils.ShellJoin(args...)
	}
	return ctx.Transport.Execute(ctx.Context, fullCmd)
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
