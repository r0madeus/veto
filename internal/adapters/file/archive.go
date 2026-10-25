package file

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

func init() {
	core.RegisterResource("archive", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewArchiveAdapter(name, params), nil
	})
	core.RegisterResource("extract", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewArchiveAdapter(name, params), nil
	})
}

type ArchiveAdapter struct {
	core.BaseResource
	Source string // Arşiv dosyasının yolu (örn: /tmp/app.zip)
	Dest   string // Nereye açılacağı (örn: /opt/app)
	Mode   os.FileMode
}

func NewArchiveAdapter(name string, params map[string]interface{}) core.Resource {
	src, _ := params["source"].(string)
	if src == "" {
		src = name
	} // Eğer source verilmezse isim source olarak kullanılır

	dest, _ := params["dest"].(string)
	if dest == "" {
		// Dest verilmezse source ile aynı yere klasör olarak aç
		dest = strings.TrimSuffix(src, filepath.Ext(src))
	}

	mode := os.FileMode(0755)
	if m, ok := params["mode"].(int); ok {
		mode = os.FileMode(m)
	} else if mDouble, ok := params["mode"].(float64); ok {
		mode = os.FileMode(int(mDouble))
	} // TODO: Handle string octal input

	return &ArchiveAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "archive"},
		Source:       src,
		Dest:         dest,
		Mode:         mode,
	}
}

func (r *ArchiveAdapter) Validate(ctx *core.SystemContext) error {
	if r.Source == "" {
		return fmt.Errorf("archive source is required")
	}
	if r.Dest == "" {
		return fmt.Errorf("archive destination is required")
	}
	return nil
}

func (r *ArchiveAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// 1. Kaynak dosya var mı?
	if _, err := ctx.FS.Stat(r.Source); os.IsNotExist(err) {
		return false, fmt.Errorf("archive source not found: %s", r.Source)
	}

	// 2. Hedef klasör var mı?
	if _, err := ctx.FS.Stat(r.Dest); os.IsNotExist(err) {
		return true, nil
	}

	// Hedef zaten varsa değişiklik yok varsayıyoruz
	return false, nil
}

func (r *ArchiveAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Archive already extracted to %s", r.Dest)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Extract %s to %s", r.Source, r.Dest)), nil
	}

	// Hedef klasörü oluştur
	if err := ctx.FS.MkdirAll(r.Dest, r.Mode); err != nil {
		return core.Failure(err, "Failed to create destination directory"), err
	}

	// Dosya uzantısına göre açma işlemi
	if strings.HasSuffix(r.Source, ".zip") {
		err = r.unzip(ctx, r.Source, r.Dest)
	} else if strings.HasSuffix(r.Source, ".tar.gz") || strings.HasSuffix(r.Source, ".tgz") {
		err = r.untar(ctx, r.Source, r.Dest)
	} else {
		return core.Failure(nil, "Unsupported archive format"), fmt.Errorf("unsupported format: %s", r.Source)
	}

	if err != nil {
		return core.Failure(err, "Extraction failed"), err
	}

	return core.SuccessChange(fmt.Sprintf("Archive extracted to %s", r.Dest)), nil
}

// --- Yardımcı Fonksiyonlar (FS uyumlu) ---

func (r *ArchiveAdapter) unzip(ctx *core.SystemContext, src, dest string) error {
	file, err := ctx.FS.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}

	for _, f := range reader.File {
		fpath := filepath.Join(dest, f.Name)

		// Zip Slip zafiyetini önle
		rel, err := filepath.Rel(dest, fpath)
		if err != nil || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
			return fmt.Errorf("illegal file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := ctx.FS.MkdirAll(fpath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err := ctx.FS.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := ctx.FS.Create(fpath) // Create uses default O_TRUNC
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}

		// Set permissions
		if err := ctx.FS.Chmod(fpath, f.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func (r *ArchiveAdapter) untar(ctx *core.SystemContext, src, dest string) error {
	file, err := ctx.FS.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Zip Slip zafiyetini önle
		rel, err := filepath.Rel(dest, target)
		if err != nil || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
			return fmt.Errorf("illegal file path: %s", target)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := ctx.FS.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := ctx.FS.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := ctx.FS.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()

			if err != nil {
				return err
			}

			// Set permissions
			if err := ctx.FS.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}
	return nil
}
