package file

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("symlink", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewSymlinkAdapter(name, params), nil
	})
}

type SymlinkAdapter struct {
	core.BaseResource
	Link   string // Linkin oluşacağı yer
	Target string // Linkin hedefi
	State  string
	Force  bool
}

func NewSymlinkAdapter(name string, params map[string]interface{}) core.Resource {
	link, _ := params["path"].(string)
	if link == "" {
		link = name
	}

	target, _ := params["target"].(string)
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	force := false
	if f, ok := params["force"].(bool); ok {
		force = f
	}

	return &SymlinkAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "symlink"},
		Link:         link,
		Target:       target,
		State:        state,
		Force:        force,
	}
}

func (r *SymlinkAdapter) Validate(ctx *core.SystemContext) error {
	if r.Link == "" || r.Target == "" {
		return fmt.Errorf("symlink requires both 'path' (link) and 'target'")
	}
	return nil
}

func (r *SymlinkAdapter) Check(ctx *core.SystemContext) (bool, error) {
	info, err := ctx.FS.Lstat(r.Link)

	if r.State == "absent" {
		return !os.IsNotExist(err), nil
	}

	if os.IsNotExist(err) {
		return true, nil
	}

	// Link değilse ve force yoksa hata ver
	if info.Mode()&os.ModeSymlink == 0 {
		if !r.Force {
			return false, fmt.Errorf("path '%s' exists and is not a symlink (use force: true to overwrite)", r.Link)
		}
		return true, nil // Force var, overwrite yapılacak
	}

	// Hedef doğru mu?
	currentDest, err := ctx.FS.Readlink(r.Link)
	if err != nil {
		return true, err
	}

	return currentDest != r.Target, nil
}

func (r *SymlinkAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Symlink %s -> %s correct", r.Link, r.Target)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Link %s -> %s (Force: %v)", r.Link, r.Target, r.Force)), nil
	}

	// Eğer dosya varsa ve force ise sil
	if _, err := ctx.FS.Lstat(r.Link); err == nil {
		if err := ctx.FS.Remove(r.Link); err != nil {
			return core.Failure(err, "Failed to remove existing path"), err
		}
	}

	if r.State == "absent" {
		return core.SuccessChange("Symlink removed"), nil
	}

	// Klasörü oluştur
	if err := ctx.FS.MkdirAll(filepath.Dir(r.Link), 0755); err != nil {
		return core.Failure(err, "Failed to create parent dir"), err
	}

	if err := ctx.FS.Symlink(r.Target, r.Link); err != nil {
		return core.Failure(err, "Failed to create symlink"), err
	}

	return core.SuccessChange(fmt.Sprintf("Symlink created: %s -> %s", r.Link, r.Target)), nil
}
