package file

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	core.RegisterResource("line_in_file", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewLineInFileAdapter(name, params), nil
	})
	core.RegisterResource("lineinfile", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewLineInFileAdapter(name, params), nil
	})
}

type LineInFileAdapter struct {
	core.BaseResource
	Path   string
	Line   string // Eklenecek/aranacak tam satır
	Regexp string // Satırı bulmak için regex (opsiyonel)
	State  string // present, absent
}

func NewLineInFileAdapter(name string, params map[string]interface{}) core.Resource {
	path, _ := params["path"].(string)
	if path == "" {
		path = name
	}

	line, _ := params["line"].(string)
	regex, _ := params["regexp"].(string)

	state, _ := params["state"].(string)
	if state == "" {
		state = "present"
	}

	return &LineInFileAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "line_in_file"},
		Path:         path,
		Line:         line,
		Regexp:       regex,
		State:        state,
	}
}

func (r *LineInFileAdapter) Validate(ctx *core.SystemContext) error {
	if r.Path == "" {
		return fmt.Errorf("path is required")
	}
	if r.Line == "" && r.State == "present" {
		return fmt.Errorf("line content is required when state is present")
	}
	return nil
}

func (r *LineInFileAdapter) Check(ctx *core.SystemContext) (bool, error) {
	content, err := ctx.FS.ReadFile(r.Path)
	if os.IsNotExist(err) {
		if r.State == "absent" {
			return false, nil
		}
		return true, nil // Dosya yoksa ve present isteniyorsa oluşturulmalı
	}
	if err != nil {
		return false, err
	}

	text := string(content)
	lines := strings.Split(text, "\n")

	found := false

	// Regex varsa regex ile ara
	if r.Regexp != "" {
		re, err := regexp.Compile(r.Regexp)
		if err != nil {
			return false, fmt.Errorf("invalid regexp: %v", err)
		}

		for _, l := range lines {
			if re.MatchString(l) {
				// Eşleşme bulundu. Peki içerik aynı mı?
				if r.State == "present" && l != r.Line {
					return true, nil // Regex uyuyor ama içerik farklı -> Değiştirilmeli
				}
				found = true
				break
			}
		}
	} else {
		// Regex yoksa düz string araması
		for _, l := range lines {
			if l == r.Line {
				found = true
				break
			}
		}
	}

	if r.State == "absent" {
		return found, nil // Varsa silinmeli -> true
	}

	return !found, nil // Yoksa eklenmeli -> true
}

func (r *LineInFileAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, _ := r.Check(ctx)
	if !needsAction {
		return core.SuccessNoChange("Line is correct"), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Update line in %s", r.Path)), nil
	}

	// Dosyayı oku (veya oluştur)
	var lines []string
	if content, err := ctx.FS.ReadFile(r.Path); err == nil {
		lines = strings.Split(string(content), "\n")
		// Son satır boşsa temizle (genellikle split boş string üretir)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	newLines := []replacedLine{}

	if r.State == "absent" {
		// Silme Modu
		for _, l := range lines {
			match := false
			if r.Regexp != "" {
				matched, _ := regexp.MatchString(r.Regexp, l)
				if matched {
					match = true
				}
			} else {
				if l == r.Line {
					match = true
				}
			}

			if !match {
				newLines = append(newLines, replacedLine{text: l})
			}
		}
	} else {
		// Ekleme/Güncelleme Modu
		replaced := false
		var re *regexp.Regexp
		if r.Regexp != "" {
			re, _ = regexp.Compile(r.Regexp)
		}

		for _, l := range lines {
			if r.Regexp != "" && re.MatchString(l) {
				// Regex eşleşti, satırı güncelle
				newLines = append(newLines, replacedLine{text: r.Line})
				replaced = true
			} else if r.Regexp == "" && l == r.Line {
				// Tam eşleşme zaten var
				newLines = append(newLines, replacedLine{text: l})
				replaced = true
			} else {
				newLines = append(newLines, replacedLine{text: l})
			}
		}

		if !replaced {
			// Eşleşme yoksa sona ekle
			newLines = append(newLines, replacedLine{text: r.Line})
		}
	}

	// Dosyaya yaz
	output := ""
	for i, nl := range newLines {
		output += nl.text
		if i < len(newLines)-1 || r.State == "present" { // Son satıra da newline ekleyelim
			output += "\n"
		}
	}

	// Sondaki newline'ı garanti et
	if len(output) > 0 && output[len(output)-1] != '\n' {
		output += "\n"
	}

	if err := ctx.FS.WriteFile(r.Path, []byte(output), 0644); err != nil {
		return core.Failure(err, "Failed to write file"), err
	}

	return core.SuccessChange("File updated"), nil
}

type replacedLine struct {
	text string
}
