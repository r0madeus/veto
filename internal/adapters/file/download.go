package file

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("download", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewDownloadAdapter(name, params), nil
	})
}

type DownloadAdapter struct {
	core.BaseResource
	URL        string
	Dest       string
	Mode       os.FileMode
	BackupPath string
}

func NewDownloadAdapter(name string, params map[string]interface{}) core.Resource {
	url, _ := params["url"].(string)
	dest, _ := params["dest"].(string)
	if dest == "" {
		dest = name // If dest not provided, maybe name is the path? Or derive from URL
	}

	mode := os.FileMode(0644)
	if m, ok := params["mode"].(int); ok {
		mode = os.FileMode(m)
	}

	return &DownloadAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "download"},
		URL:          url,
		Dest:         dest,
		Mode:         mode,
	}
}

func (r *DownloadAdapter) Validate(ctx *core.SystemContext) error {
	if r.URL == "" {
		return fmt.Errorf("download url is required")
	}
	if r.Dest == "" {
		return fmt.Errorf("destination path is required")
	}
	return nil
}

func (r *DownloadAdapter) Check(ctx *core.SystemContext) (bool, error) {
	info, err := ctx.FS.Stat(r.Dest)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode().Perm() != r.Mode {
		return true, nil
	}

	return false, nil
}

func (r *DownloadAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("File %s already exists", r.Dest)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Download %s to %s", r.URL, r.Dest)), nil
	}

	// YEDEKLEME
	if ctx.BackupManager != nil && ctx.TxID != "" {
		backupPath, err := ctx.BackupManager.CreateBackup(ctx.TxID, r.Dest)
		if err == nil {
			r.BackupPath = backupPath
		}
	}

	// Ensure dir exists
	if err := ctx.FS.MkdirAll(filepath.Dir(r.Dest), 0755); err != nil {
		return core.Failure(err, "Failed to create directory"), err
	}

	resp, err := http.Get(r.URL)
	if err != nil {
		return core.Failure(err, "Download request failed"), err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return core.Failure(nil, fmt.Sprintf("Download failed with status: %s", resp.Status)), fmt.Errorf("status %s", resp.Status)
	}

	out, err := ctx.FS.Create(r.Dest)
	if err != nil {
		return core.Failure(err, "Failed to create file"), err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return core.Failure(err, "Failed to write file"), err
	}

	if err := ctx.FS.Chmod(r.Dest, r.Mode); err != nil {
		return core.Failure(err, "Failed to set permissions"), err
	}

	return core.SuccessChange(fmt.Sprintf("Downloaded %s to %s", r.URL, r.Dest)), nil
}

func (r *DownloadAdapter) Revert(ctx *core.SystemContext) error {
	if r.BackupPath != "" {
		return core.CopyFile(ctx.FS, r.BackupPath, r.Dest, r.Mode)
	}
	// Yedek yoksa yeni indirilmiştir, sil
	return ctx.FS.Remove(r.Dest)
}
