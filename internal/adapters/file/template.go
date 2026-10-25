package file

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("template", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewTemplateAdapter(name, params), nil
	})
}

type TemplateAdapter struct {
	core.BaseResource
	Src        string                 // Template dosyasının yolu
	Dest       string                 // Çıktı dosyasının yolu
	Vars       map[string]interface{} // Template içine gönderilecek değişkenler
	Mode       os.FileMode
	BackupPath string
}

func NewTemplateAdapter(name string, params map[string]interface{}) core.Resource {
	src, _ := params["src"].(string)
	dest, _ := params["dest"].(string)
	if dest == "" {
		dest = name
	}

	vars := make(map[string]interface{})
	if v, ok := params["vars"].(map[string]interface{}); ok {
		vars = v
	}

	mode := os.FileMode(0644)
	if m, ok := params["mode"].(int); ok {
		mode = os.FileMode(m)
	}

	return &TemplateAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "template"},
		Src:          src,
		Dest:         dest,
		Vars:         vars,
		Mode:         mode,
	}
}

func (r *TemplateAdapter) Validate(ctx *core.SystemContext) error {
	if r.Src == "" {
		return fmt.Errorf("template source 'src' is required")
	}
	if r.Dest == "" {
		return fmt.Errorf("template destination 'dest' is required")
	}

	// Template dosyasının varlığını kontrol et
	if _, err := ctx.FS.Stat(r.Src); os.IsNotExist(err) {
		return fmt.Errorf("template source file '%s' does not exist", r.Src)
	}

	return nil
}

func (r *TemplateAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// 1. Hedef dosya yoksa -> Değişiklik var
	if _, err := ctx.FS.Stat(r.Dest); os.IsNotExist(err) {
		return true, nil
	}

	// 2. İçerik kontrolü: Template'i bellekte render et ve mevcut dosyayla karşılaştır
	rendered, err := r.render(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to render template for check: %w", err)
	}

	currentContent, err := ctx.FS.ReadFile(r.Dest)
	if err != nil {
		return false, err
	}

	if string(currentContent) != rendered {
		return true, nil
	}
	return false, nil
}

func (r *TemplateAdapter) Diff(ctx *core.SystemContext) (string, error) {
	rendered, err := r.render(ctx)
	if err != nil {
		return "", err
	}

	current := ""
	if _, err := ctx.FS.Stat(r.Dest); err == nil {
		c, _ := ctx.FS.ReadFile(r.Dest)
		current = string(c)
	}

	return core.GenerateDiff(r.Dest, current, rendered), nil
}

func (r *TemplateAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Template check failed"), err
	}
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Template %s is up to date", r.Dest)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Render template %s to %s", r.Src, r.Dest)), nil
	}

	// YEDEKLEME
	if ctx.BackupManager != nil && ctx.TxID != "" {
		backupPath, err := ctx.BackupManager.CreateBackup(ctx.TxID, r.Dest)
		if err == nil {
			r.BackupPath = backupPath
		}
	}

	// Render et
	content, err := r.render(ctx)
	if err != nil {
		return core.Failure(err, "Template render failed"), err
	}

	// Klasörü oluştur
	if err := ctx.FS.MkdirAll(filepath.Dir(r.Dest), 0755); err != nil {
		return core.Failure(err, "Failed to create directory"), err
	}

	// Yaz
	if err := ctx.FS.WriteFile(r.Dest, []byte(content), r.Mode); err != nil {
		return core.Failure(err, "Failed to write file"), err
	}

	return core.SuccessChange("Template applied successfully"), nil
}

func (r *TemplateAdapter) Revert(ctx *core.SystemContext) error {
	if r.BackupPath != "" {
		return core.CopyFile(ctx.FS, r.BackupPath, r.Dest, r.Mode)
	}
	return ctx.FS.Remove(r.Dest)
}

func (r *TemplateAdapter) render(ctx *core.SystemContext) (string, error) {
	// Template dosyasını oku
	tmplContent, err := ctx.FS.ReadFile(r.Src)
	if err != nil {
		return "", err
	}

	// Parse et
	t, err := template.New(filepath.Base(r.Src)).Parse(string(tmplContent))
	if err != nil {
		return "", err
	}

	// Execute et (Buffer'a yaz)
	var buf bytes.Buffer
	if err := t.Execute(&buf, r.Vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
