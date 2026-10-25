package font

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/utils"
)

func init() {
	core.RegisterResource("font", NewFontAdapter)
}

type FontAdapter struct {
	Name   string // e.g. "JetBrainsMono"
	Source string // URL to zip/tar
	System bool   // System-wide install?
	Params map[string]interface{}
}

func NewFontAdapter(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
	source, _ := params["source"].(string)
	system, _ := params["system"].(bool)

	return &FontAdapter{
		Name:   name,
		Source: source,
		System: system,
		Params: params,
	}, nil
}

func (a *FontAdapter) GetName() string { return a.Name }
func (a *FontAdapter) GetType() string { return "font" }

func (a *FontAdapter) Validate(ctx *core.SystemContext) error {
	if a.Source == "" {
		return fmt.Errorf("source url is required for font %s", a.Name)
	}
	return nil
}

func (a *FontAdapter) getDestPath(ctx *core.SystemContext) string {
	if a.System {
		return filepath.Join("/usr/share/fonts", a.Name)
	}
	// User install: ~/.local/share/fonts
	// We use ctx.HomeDir which should be populated for target user
	return filepath.Join(ctx.HomeDir, ".local/share/fonts", a.Name)
}

func (a *FontAdapter) Check(ctx *core.SystemContext) (bool, error) {
	dest := a.getDestPath(ctx)

	// Check if destination directory exists
	// Using generic "test -d" via shell because Transport doesn't have explicit DirExists yet (stat works too)
	// Transport.Stat returns interface{}

	// Better: Use Transport.GetFileSystem().Stat
	_, err := ctx.Transport.GetFileSystem().Stat(dest)
	if err == nil {
		// Compatible with existing logic: if exists, we assume it's correctly installed
		// TODO: Validate version if possible?
		return false, nil // No change needed
	}

	// If error is NotExist, then we need to install
	return true, nil
}

func (a *FontAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	dest := a.getDestPath(ctx)

	// 1. Download to LOCAL temp first (Controller)
	// 1. Download to LOCAL temp first (Controller)
	// Use unique temp file to avoid collision if local == remote path
	f, err := os.CreateTemp("", fmt.Sprintf("veto_%s_*.zip", a.Name))
	if err != nil {
		return core.Failure(err, "Failed to create temp file"), err
	}
	tmpLocal := f.Name()
	f.Close() // Close immediately, DownloadFile will reopen/write or overwrite

	if err := utils.DownloadFile(a.Source, tmpLocal); err != nil {
		return core.Failure(err, "Failed to download font"), err
	}
	defer os.Remove(tmpLocal)

	// 2. Transfer to REMOTE temp
	tmpRemote := fmt.Sprintf("/tmp/veto_%s.zip", a.Name)
	if err := ctx.Transport.CopyFile(ctx.Context, tmpLocal, tmpRemote); err != nil {
		return core.Failure(err, "Failed to copy font to target"), err
	}

	// 3. Prepare Commands
	// Ensure unzip installed?
	// Extract
	// We create a temp dir to extract
	tmpExtract := fmt.Sprintf("/tmp/veto_extract_%s", a.Name)

	cmds := []string{
		fmt.Sprintf("mkdir -p %s", utils.ShellQuote(tmpExtract)),
		fmt.Sprintf("unzip -o %s -d %s", utils.ShellQuote(tmpRemote), utils.ShellQuote(tmpExtract)),
		fmt.Sprintf("mkdir -p %s", utils.ShellQuote(dest)),
		fmt.Sprintf("cp -r %s/* %s/", utils.ShellQuote(tmpExtract), utils.ShellQuote(dest)),
		fmt.Sprintf("rm -rf %s %s", utils.ShellQuote(tmpExtract), utils.ShellQuote(tmpRemote)),
	}

	// If system install, we might need sudo.
	// But Veto runs as current user (or root).
	// If user allows sudo via SSH config (BecomeMethod), Execute might handle it?
	// Currently Engine doesn't auto-wrap simple Execute commands with sudo.
	// SSHTransport.Execute is raw.
	// Users should run Veto as root or adequate permissions for system install.

	for _, cmd := range cmds {
		if ctx.DryRun {
			continue // Skip actual execution
		}
		if _, err := ctx.Transport.Execute(ctx.Context, cmd); err != nil {
			return core.Failure(err, fmt.Sprintf("Failed to install font: %s", cmd)), err
		}
	}

	// 4. Update Cache
	if !ctx.DryRun {
		ctx.Transport.Execute(ctx.Context, "fc-cache -f")
	}

	return core.SuccessChange(fmt.Sprintf("Font %s installed to %s", a.Name, dest)), nil
}
