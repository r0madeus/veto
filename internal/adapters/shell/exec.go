package shell

import (
	"fmt"

	"github.com/r0madeus/veto/internal/core"
)

func init() {
	factory := func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewExecAdapter(name, params), nil
	}
	core.RegisterResource("exec", factory)
	core.RegisterResource("shell", factory)
	core.RegisterResource("cmd", factory)
}

type ExecAdapter struct {
	core.BaseResource
	Command       string
	Unless        string // Eğer bu komut başarılı olursa (exit 0), ana komutu çalıştırma
	OnlyIf        string // Sadece bu komut başarılı olursa ana komutu çalıştır
	RevertCommand string // Rollback durumunda çalıştırılacak komut
}

func NewExecAdapter(name string, params map[string]interface{}) core.Resource {
	cmd, _ := params["command"].(string)
	if cmd == "" {
		cmd = name
	} // Command verilmezse isim komut olarak kullanılır

	unless, _ := params["unless"].(string)
	onlyif, _ := params["onlyif"].(string)
	revertCmd, _ := params["revert_command"].(string)

	return &ExecAdapter{
		BaseResource:  core.BaseResource{Name: name, Type: "exec"},
		Command:       cmd,
		Unless:        unless,
		OnlyIf:        onlyif,
		RevertCommand: revertCmd,
	}
}

func (r *ExecAdapter) Validate(ctx *core.SystemContext) error {
	if r.Command == "" {
		return fmt.Errorf("command is required for exec resource")
	}
	return nil
}

func (r *ExecAdapter) Check(ctx *core.SystemContext) (bool, error) {
	// 1. Unless Kontrolü: Eğer başarılı olursa, çalışmaya GEREK YOK (return false)
	if r.Unless != "" {
		_, err := ctx.Transport.Execute(ctx.Context, r.Unless)
		if err == nil {
			return false, nil // Unless başarılı, işlem yapma
		}
	}

	// 2. OnlyIf Kontrolü: Eğer başarısız olursa, çalışmaya GEREK YOK (return false)
	if r.OnlyIf != "" {
		_, err := ctx.Transport.Execute(ctx.Context, r.OnlyIf)
		if err != nil {
			return false, nil // OnlyIf başarısız, işlem yapma
		}
	}

	// Exec her zaman "değişiklik yapacak" kabul edilir (unless/onlyif yoksa)
	return true, nil
}

func (r *ExecAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	shouldRun, _ := r.Check(ctx)
	if !shouldRun {
		return core.SuccessNoChange("Skipped due to unless/onlyif conditions"), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Exec: %s", r.Command)), nil
	}

	out, err := ctx.Transport.Execute(ctx.Context, r.Command)
	if err != nil {
		return core.Failure(err, fmt.Sprintf("Command failed: %s", out)), err
	}

	return core.SuccessChange("Command executed successfully"), nil
}

func (r *ExecAdapter) Revert(ctx *core.SystemContext) error {
	if r.RevertCommand != "" {
		out, err := ctx.Transport.Execute(ctx.Context, r.RevertCommand)
		if err != nil {
			return fmt.Errorf("revert command failed: %s: %w", out, err)
		}
	}
	return nil
}
