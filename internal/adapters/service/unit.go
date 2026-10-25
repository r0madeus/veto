package service

import (
	"fmt"
	"github.com/r0madeus/veto/internal/utils"
	"path/filepath"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("systemd_unit", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewSystemdUnitAdapter(name, params), nil
	})
}

type SystemdUnitAdapter struct {
	core.BaseResource
	Content         string
	Source          string
	Path            string
	State           string
	ActionPerformed string
}

func NewSystemdUnitAdapter(name string, params map[string]interface{}) core.Resource {
	content, _ := params["content"].(string)
	source, _ := params["source"].(string)
	path, _ := params["path"].(string)

	if path == "" {
		// Default systemd unit path
		path = "/etc/systemd/system/" + name
	} else if !strings.HasSuffix(path, ".service") && !strings.HasSuffix(path, ".timer") && !strings.HasSuffix(path, ".socket") {
		// If path is a dir, join name
		path = filepath.Join(path, name)
	}

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	return &SystemdUnitAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "systemd_unit"},
		Content:      content,
		Source:       source,
		Path:         path,
		State:        state,
	}
}

func (r *SystemdUnitAdapter) Validate(ctx *core.SystemContext) error {
	if r.Name == "" {
		return fmt.Errorf("unit name is required")
	}
	if r.Content == "" && r.Source == "" && r.State == "present" {
		return fmt.Errorf("either 'content' or 'source' must be specified for systemd_unit")
	}
	return nil
}

func (r *SystemdUnitAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// Check if file exists
	fs := ctx.FS
	if ctx.Transport != nil {
		fs = ctx.Transport.GetFileSystem()
	}

	exists := false
	if _, err := fs.Stat(r.Path); err == nil {
		exists = true
	}

	if r.State == "absent" {
		return exists, nil
	}

	if !exists {
		return true, nil // Needs creation
	}

	// File exists, check content
	currentContentBytes, err := fs.ReadFile(r.Path)
	if err != nil {
		return false, fmt.Errorf("failed to read existing unit file: %w", err)
	}
	currentContent := string(currentContentBytes)

	// Determine desired content
	desiredContent := r.Content
	if r.Source != "" {
		// If source is local file, read it?
		// Or if source is provided, we assume it might be complex
		// For now simple content string is preferred.
		// If source is used, we might need to read it.
		// Let's assume content acts as primary if both set? Or error?
		if desiredContent == "" {
			// Read source file using local FS (where veto runs) NOT remote
			// BUT if this is remote execution, source is on local machine.
			// SystemContext has reference to "Local" FS? Or "Source" FS?
			// Engine doesn't pass that easily.
			// Let's assume 'content' is the way for templated strings.
			// 'source' might be for copying file.
			// If source, we rely on CopyFile checksums... but Transport.CopyFile doesn't return Changed bool easily
			// unless we implement check manually.
			// For simplicity: We recommend 'content'. If 'source' is used, we blindly copy or check hash.
			return true, nil // TODO: Implement source comparison
		}
	}

	// Normalize comparison (trim space)
	if strings.TrimSpace(currentContent) != strings.TrimSpace(desiredContent) {
		return true, nil // Content mismatch
	}

	return false, nil
}

func (r *SystemdUnitAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Unit %s is up-to-date", r.Name)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Unit %s -> %s", r.Name, r.State)), nil
	}

	fs := ctx.FS
	if ctx.Transport != nil {
		fs = ctx.Transport.GetFileSystem()
	}

	if r.State == "absent" {
		if err := fs.Remove(r.Path); err != nil {
			return core.Failure(err, "Failed to remove unit file"), err
		}
		// Daemon Reload
		if _, err := ctx.Transport.Execute(ctx.Context, "systemctl daemon-reload"); err != nil {
			ctx.Logger.Warn("systemctl daemon-reload failed after unit removal: %v", err)
		}

		r.ActionPerformed = "removed"
		return core.SuccessChange("Unit removed"), nil
	}

	// Write Content
	if r.Content != "" {
		// Using Transport.Execute for echo/tee might be safer for sudo write,
		// but using FileSystem abstraction if it supports it.
		// Abstract FS usually runs as current user. If target is /etc/systemd/system, we need sudo.
		// Transport.CopyFile handles sudo if implemented?
		// Or we use "echo ... | sudo tee ..."

		// If we are root (local), FS.WriteFile works.
		// If we are not root, we need sudo.
		// Let's try direct write first, if fails try sudo tee via Execute?
		// Better: use a helper WriteFilePrivileged(ctx, path, content)

		err := fs.WriteFile(r.Path, []byte(r.Content), 0644)
		if err != nil {
			// Fallback to sudo tee if permission denied?
			// Simple heuristics:
			ctx.Logger.Debug("Direct write failed, trying sudo tee...", "err", err)

			// Escape content for shell? This is dangerous.
			// Safer: Write to temp, then sudo mv.
			tmpPath := "/tmp/" + r.Name + ".tmp"
			if wErr := fs.WriteFile(tmpPath, []byte(r.Content), 0644); wErr != nil {
				return core.Failure(wErr, "Failed to write temp file"), wErr
			}

			cmd := fmt.Sprintf("sudo mv %s %s && sudo chown root:root %s && sudo chmod 644 %s", utils.ShellQuote(tmpPath), utils.ShellQuote(r.Path), utils.ShellQuote(r.Path), utils.ShellQuote(r.Path))
			if _, eErr := ctx.Transport.Execute(ctx.Context, cmd); eErr != nil {
				return core.Failure(eErr, "Failed to move unit file via sudo"), eErr
			}
		}
	} else if r.Source != "" {
		if err := ctx.Transport.CopyFile(ctx.Context, r.Source, r.Path); err != nil {
			return core.Failure(err, "Failed to copy unit file"), err
		}
	}

	// Daemon Reload
	if _, err := ctx.Transport.Execute(ctx.Context, "systemctl daemon-reload"); err != nil {
		return core.Failure(err, "systemctl daemon-reload failed"), err
	}

	r.ActionPerformed = "created"
	return core.SuccessChange("Unit file created/updated & daemon reloaded"), nil
}

func (r *SystemdUnitAdapter) Revert(ctx *core.SystemContext) error {
	if r.ActionPerformed == "created" {
		// Remove file
		_, _ = ctx.Transport.Execute(ctx.Context, "rm "+utils.ShellQuote(r.Path)) // Try sudo rm if needed
		_, _ = ctx.Transport.Execute(ctx.Context, "systemctl daemon-reload")
	}
	return nil
}
