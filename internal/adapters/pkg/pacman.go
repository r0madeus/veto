package pkg

import (
	"fmt"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

// PacmanAdapter, Arch Linux pacman paket yöneticisi için adaptör.
type PacmanAdapter struct {
	core.BaseResource        // Ortak alanlar (Name, Type) buradan gelir
	State             string // "present", "absent"
	ActionPerformed   string // "installed", "removed", ""
}

func init() {
	core.RegisterResource("pacman", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewPacmanAdapter(name, params), nil
	})
}

// NewPacmanAdapter yeni bir örnek oluşturur.
func NewPacmanAdapter(name string, params map[string]interface{}) core.Resource {
	pkgName, _ := params["name"].(string)
	if pkgName == "" {
		pkgName = name
	}

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}
	return &PacmanAdapter{
		BaseResource: core.BaseResource{Name: pkgName, Type: "package"},
		State:        state,
	}
}

func (r *PacmanAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("package name is required")
	}
	if r.State != "present" && r.State != "absent" {
		return fmt.Errorf("invalid state '%s', must be 'present' or 'absent'", r.State)
	}
	return nil
}

func (r *PacmanAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// Pacman ile paket kontrolü: pacman -Qi <paket>
	installed := isInstalled(ctx, "pacman", "-Qi", r.Name)

	if r.State == "absent" {
		// Eğer silinmesi isteniyorsa ama yüklüyse -> Değişiklik lazım (true)
		return installed, nil
	}

	// Eğer yüklenmesi isteniyorsa (present) ve yüklü değilse -> Değişiklik lazım (true)
	return !installed, nil
}

func (r *PacmanAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	// Önce durum kontrolü yap (Dry-run desteği için önemli)
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Failed to check package status"), err
	}

	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Package %s is already in desired state (%s)", r.Name, r.State)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Would %s package %s", r.State, r.Name)), nil
	}

	// İşlemi Gerçekleştir
	var cmd string
	var args []string

	if r.State == "absent" {
		// Kaldır: pacman -Rns --noconfirm
		cmd = "pacman"
		args = []string{"-Rns", "--noconfirm", r.Name}
		r.ActionPerformed = "removed"
	} else {
		// Kur: pacman -S --noconfirm --needed
		cmd = "pacman"
		args = []string{"-S", "--noconfirm", "--needed", r.Name}
		r.ActionPerformed = "installed"
	}

	output, err := runCommand(ctx, cmd, args...)
	if err != nil {
		r.ActionPerformed = ""
		return core.Failure(err, fmt.Sprintf("Failed to %s package %s: %s", r.State, r.Name, output)), err
	}

	return core.SuccessChange(fmt.Sprintf("Successfully %s package %s", r.State, r.Name)), nil
}

func (r *PacmanAdapter) Revert(ctx *core.SystemContext) error {
	return r.RevertAction("installed", ctx)
}

func (r *PacmanAdapter) RevertAction(action string, ctx *core.SystemContext) error {
	// Revert the given action
	if action == "installed" {
		// Undo install -> Remove
		_, err := runCommand(ctx, "pacman", "-Rns", "--noconfirm", r.Name)
		return err
	} else if action == "removed" {
		// Undo remove -> Install
		_, err := runCommand(ctx, "pacman", "-S", "--noconfirm", "--needed", r.Name)
		return err
	}
	return fmt.Errorf("unknown action to revert: %s", action)
}

// ListInstalled returns a list of explicitly installed packages.
func (r *PacmanAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	// pacman -Qqe: Query, Quiet (name only), Explicitly installed
	output, err := runCommand(ctx, "pacman", "-Qqe")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}

	// runCommand returns string.
	// We need strings package.
	// But let's verify if runCommand output is clean. Yes standard stdout.
	return splitLines(output), nil
}

// RemoveBatch removes multiple packages at once.
func (r *PacmanAdapter) RemoveBatch(names []string, ctx *core.SystemContext) error {
	if len(names) == 0 {
		return nil
	}

	if ctx.DryRun {
		ctx.Logger.Info("[DryRun] Would remove packages: %s", strings.Join(names, ", "))
		return nil
	}

	args := []string{"-Rns", "--noconfirm"}
	args = append(args, names...)

	output, err := runCommand(ctx, "pacman", args...)
	if err != nil {
		return fmt.Errorf("failed to batch remove packages: %s: %w", output, err)
	}

	return nil
}
