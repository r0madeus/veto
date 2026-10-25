package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("file", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewFileAdapter(name, params), nil
	})
}

type FileAdapter struct {
	core.BaseResource
	Path            string
	Source          string // Kopyalanacak kaynak dosya (opsiyonel)
	Content         string // Yazılacak içerik (opsiyonel)
	Method          string // copy (default), symlink
	Mode            os.FileMode
	State           string // present, absent
	BackupPath      string // Yedeklenen dosyanın yolu
	Prune           bool   // Dizin için: konfikte olmayan dosyaları sil
	ActionPerformed string // created, modified, deleted
}

func (r *FileAdapter) GetBackupPath() string {
	return r.BackupPath
}

func NewFileAdapter(name string, params map[string]interface{}) core.Resource {
	path, _ := params["path"].(string)
	if path == "" {
		path = name // Eğer path verilmezse name'i path olarak kullan
	}

	source, _ := params["source"].(string)
	content, _ := params["content"].(string)
	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	// İzinleri ayarla (varsayılan 0644)
	mode := os.FileMode(0644)
	if m, ok := params["mode"].(int); ok {
		mode = os.FileMode(m)
	}

	method, _ := params["method"].(string)
	if method == "" {
		method = "copy"
	}

	backupPath, _ := params["backup_path"].(string)
	prune, _ := params["prune"].(bool)

	return &FileAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "file"},
		Path:         path,
		Source:       source,
		Content:      content,
		Method:       method,
		Mode:         mode,
		State:        state,
		BackupPath:   backupPath,
		Prune:        prune,
	}
}

// RevertAction implements the Revertable interface for smart rollback
func (r *FileAdapter) RevertAction(action string, ctx *core.SystemContext) error {
	// For files, "applied" usually means created or modified.
	// If we have a backup, restore it.
	if r.BackupPath != "" {
		ctx.Logger.Info("Restoring backup from %s to %s", r.BackupPath, r.Path)
		if ctx.BackupManager != nil {
			// Try to cast to an interface that supports RestoreBackup or specific implementation
			// Since generic BackupManager interface in core might not have RestoreBackup?
			// Checking core/context.go for BackupManager interface definition.
			// Assuming we add RestoreBackup to interface.

			// For now, if we cannot change interface easily without cycle or refactor,
			// we stick to direct copy BUT we should use the Mock properly in test.
			// The issue in test is that test logic expects Mock callback, but code calls r.copyFile which does FS op.

			// We should try to use the BackupManager if it supports Restore.
			if bm, ok := ctx.BackupManager.(interface {
				RestoreBackup(string, string) error
			}); ok {
				return bm.RestoreBackup(r.BackupPath, r.Path)
			}
		}
		// Fallback copy
		return r.copyFile(ctx, r.BackupPath, r.Path, r.Mode)
	}

	// If no backup, and action was "applied" (created/modified):
	// If it was created (previously absent), we should delete it?
	// But we don't know if it was created or modified unless history says so.
	// History says "Action: applied".
	// If pre-state was "absent", then we delete.
	// TransactionChange structure doesn't easily show pre-state without loading full Transaction.
	// But let's assume if no backup, we can't safely revert modification.
	// However, if we follow the rule: "If file rollback logic", we blindly revert to "absent" if no backup? No, dangerous.
	// Safe bet: Only restore if backup exists. Or if we explicitely know it was "created".
	// For now, rely on backup.
	if action == "applied" && r.BackupPath == "" {
		ctx.Logger.Warn("No backup found for %s. Skipping rollback of modification.", r.Path)
		return nil
	}

	if action == "created" {
		ctx.Logger.Info("Reverting creation of %s (deleting)", r.Path)
		return ctx.FS.Remove(r.Path)
	}

	return nil
}

func (r *FileAdapter) Validate(ctx *core.SystemContext) error {
	if r.Path == "" {
		return fmt.Errorf("file path is required")
	}

	if r.State != "present" && r.State != "absent" {
		return fmt.Errorf("invalid state '%s', must be 'present' or 'absent'", r.State)
	}

	if r.State == "present" {
		if r.Source == "" && r.Content == "" && !r.Prune {
			return fmt.Errorf("either 'source' or 'content' must be provided for file resource when state is 'present' (unless prune is true)")
		}
		if r.Source != "" && r.Content != "" {
			return fmt.Errorf("cannot provide both 'source' and 'content' for file resource")
		}
		if r.Source != "" {
			// Source check - usually repo-side, but let's use ctx.FS if we assume a unified view
			if _, err := ctx.FS.Stat(r.Source); os.IsNotExist(err) {
				return fmt.Errorf("source file '%s' does not exist", r.Source)
			}
		}
		if r.Method != "copy" && r.Method != "symlink" {
			return fmt.Errorf("invalid method '%s', must be 'copy' or 'symlink'", r.Method)
		}
	}

	return nil
}

func (r *FileAdapter) Check(ctx *core.SystemContext) (bool, error) {
	info, err := ctx.FS.Stat(r.Path)

	if r.State == "absent" {
		// Dosya varsa silinmeli -> değişiklik var (true)
		return !os.IsNotExist(err), nil
	}

	// Dosya yoksa oluşturulmalı -> değişiklik var
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// İzin kontrolü
	if info.Mode().Perm() != r.Mode {
		return true, nil
	}

	// İçerik kontrolü
	if r.Content != "" {
		existingContent, err := ctx.FS.ReadFile(r.Path)
		if err != nil {
			return false, err
		}
		if string(existingContent) != r.Content {
			return true, nil
		}
	} else if r.Source != "" {
		if r.Method == "symlink" {
			// Check if it is a symlink and points to correct source
			if info.Mode()&os.ModeSymlink == 0 {
				return true, nil // It's not a symlink
			}

			linkDest, err := ctx.FS.Readlink(r.Path)
			if err != nil {
				return false, err
			}

			// Resolve paths for safe comparison
			absSource, _ := filepath.Abs(r.Source)
			absDest, _ := filepath.Abs(linkDest)

			if absDest != absSource {
				return true, nil
			}
		} else {
			// Copy Mode
			// Source ile hedefi karşılaştır
			same, err := r.compareFiles(ctx, r.Source, r.Path)
			if err != nil {
				return false, err
			}
			if !same {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *FileAdapter) Diff(ctx *core.SystemContext) (string, error) {
	if r.State == "absent" {
		if _, err := ctx.FS.Stat(r.Path); os.IsNotExist(err) {
			return "", nil
		}
		current, _ := ctx.FS.ReadFile(r.Path)
		return core.GenerateDiff(r.Path, string(current), ""), nil
	}

	// For "present"
	var desired string
	if r.Content != "" {
		desired = r.Content
	} else if r.Source != "" {
		s, err := ctx.FS.ReadFile(r.Source)
		if err != nil {
			return "", err
		}
		desired = string(s)
	}

	current := ""
	if _, err := ctx.FS.Stat(r.Path); err == nil {
		c, _ := ctx.FS.ReadFile(r.Path)
		current = string(c)
	}

	return core.GenerateDiff(r.Path, current, desired), nil
}

func (r *FileAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("File %s is up to date", r.Path)), nil
	}

	if ctx.DryRun {
		msg := fmt.Sprintf("[DryRun] Would %s file %s", r.State, r.Path)
		if r.Content != "" {
			msg += fmt.Sprintf("\nContent Preview:\n%s", r.Content)
		}
		return core.SuccessChange(msg), nil
	}

	// Check existence to determine action type (created vs modified)
	exists := false
	if _, err := ctx.FS.Stat(r.Path); err == nil {
		exists = true
	}

	// YEDEKLEME
	if ctx.BackupManager != nil && ctx.TxID != "" {
		ctx.Logger.Debug("[%s] Creating backup of %s", r.Name, r.Path)
		backupPath, err := ctx.BackupManager.CreateBackup(ctx.TxID, r.Path)
		if err == nil {
			r.BackupPath = backupPath
			ctx.Logger.Trace("[%s] Backup created at %s", r.Name, backupPath)
		} else {
			return core.Failure(err, "Failed to backup file"), err
		}
	}

	if r.State == "absent" {
		if err := ctx.FS.Remove(r.Path); err != nil {
			return core.Failure(err, "Failed to delete file"), err
		}
		r.ActionPerformed = "deleted"
		return core.SuccessChange("File deleted"), nil
	}

	// Dizin yoksa oluştur
	dir := filepath.Dir(r.Path)
	if err := ctx.FS.MkdirAll(dir, 0755); err != nil {
		return core.Failure(err, "Failed to create directory"), err
	}

	// İçerik yazma veya kopyalama
	if r.Content != "" {
		if err := ctx.FS.WriteFile(r.Path, []byte(r.Content), r.Mode); err != nil {
			return core.Failure(err, "Failed to write content"), err
		}
	} else if r.Source != "" {
		if r.Method == "symlink" {
			// Delete existing if present (since we confirmed it's wrong in Check)
			ctx.FS.Remove(r.Path)

			// Create symlink
			if err := ctx.FS.Symlink(r.Source, r.Path); err != nil {
				return core.Failure(err, "Failed to create symlink"), err
			}
		} else {
			if err := r.copyFile(ctx, r.Source, r.Path, r.Mode); err != nil {
				return core.Failure(err, "Failed to copy file"), err
			}
		}
	}

	if r.BackupPath != "" {
		r.ActionPerformed = "modified"
	} else {
		if exists {
			r.ActionPerformed = "modified"
		} else {
			r.ActionPerformed = "created"
		}
	}

	return core.SuccessChange(fmt.Sprintf("File %s created/updated", r.Path)), nil
}

func (r *FileAdapter) Revert(ctx *core.SystemContext) error {
	if r.BackupPath != "" {
		// Yedeği geri yükle
		return r.copyFile(ctx, r.BackupPath, r.Path, r.Mode)
	}

	// Only delete if we specifically created it
	if r.ActionPerformed == "created" {
		ctx.Logger.Info("Reverting creation of %s (deleting)", r.Path)
		return ctx.FS.Remove(r.Path)
	}

	if r.ActionPerformed == "modified" {
		ctx.Logger.Warn("Cannot revert modification of %s: no backup found.", r.Path)
		return nil
	}

	// Fallback for legacy/unknown state: Do nothing is safer than Delete
	return nil
}

// copyFile basitleştirilmiş kopyalama fonksiyonu (FS üzerinden)
func (r *FileAdapter) copyFile(ctx *core.SystemContext, src, dst string, mode os.FileMode) error {
	sourceFile, err := ctx.FS.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := ctx.FS.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}
	return ctx.FS.Chmod(dst, mode)
}

// compareFiles logic moved to FileAdapter to use FS abstraction
func (r *FileAdapter) compareFiles(ctx *core.SystemContext, src, dst string) (bool, error) {
	s, err := ctx.FS.ReadFile(src)
	if err != nil {
		return false, err
	}
	d, err := ctx.FS.ReadFile(dst)
	if err != nil {
		return false, err
	}
	return string(s) == string(d), nil
}

// ListInstalled satisfies the core.Lister interface.
// For files, this is only meaningful if the resource points to a directory and has Prune: true.
func (r *FileAdapter) ListInstalled(ctx *core.SystemContext) ([]string, error) {
	if !r.Prune {
		return nil, nil
	}

	info, err := ctx.FS.Stat(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if !info.IsDir() {
		// If it's a single file, we don't "list" it for pruning in the traditional sense
		// unless we want to support "ensure this exact file is the ONLY one"?
		// No, scoped pruning is for directories.
		return nil, nil
	}

	// List files in directory
	entries, err := ctx.FS.ReadDir(r.Path)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		files = append(files, filepath.Join(r.Path, entry.Name()))
	}

	return files, nil
}
