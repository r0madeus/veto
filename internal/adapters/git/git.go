package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/utils"
)

func init() {
	core.RegisterResource("git", func(name string, params map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		return NewGitAdapter(name, params), nil
	})
}

type GitAdapter struct {
	core.BaseResource
	Repo        string
	Dest        string
	Branch      string
	Tag         string
	Commit      string
	Remote      string
	Update      bool
	State       string // present (clone/pull), absent (delete)
	PreviousSHA string // Rollback için
	IsNew       bool   // Yeni klonlandı mı?
}

func NewGitAdapter(name string, params map[string]interface{}) core.Resource {
	repo, _ := params["repo"].(string)
	dest, _ := params["dest"].(string)
	branch, _ := params["branch"].(string)
	tag, _ := params["tag"].(string)
	commit, _ := params["commit"].(string)
	state, _ := params["state"].(string)

	update := false
	if u, ok := params["update"].(bool); ok {
		update = u
	}

	remote, _ := params["remote"].(string)
	if remote == "" {
		remote = "origin"
	}

	if state == "" {
		state = "present"
	}
	if branch == "" && tag == "" && commit == "" {
		branch = "main" // Default
	}

	return &GitAdapter{
		BaseResource: core.BaseResource{Name: name, Type: "git"},
		Repo:         repo,
		Dest:         dest,
		Branch:       branch,
		Tag:          tag,
		Commit:       commit,
		Remote:       remote,
		Update:       update,
		State:        state,
	}
}

func (r *GitAdapter) Validate(ctx *core.SystemContext) error {
	if r.Repo == "" {
		return fmt.Errorf("git repository url is required")
	}
	if r.Dest == "" {
		return fmt.Errorf("git destination path is required")
	}
	if !utils.IsOneOf(r.State, "present", "absent") {
		return fmt.Errorf("invalid state '%s': must be one of [present, absent]", r.State)
	}
	// A leading '-' would be parsed by git as an option, not a value.
	for field, v := range map[string]string{"repo": r.Repo, "remote": r.Remote, "branch": r.Branch, "tag": r.Tag, "commit": r.Commit} {
		if strings.HasPrefix(v, "-") {
			return fmt.Errorf("invalid %s '%s': must not start with '-'", field, v)
		}
	}
	return nil
}

func (r *GitAdapter) Check(ctx *core.SystemContext) (bool, error) {
	if r.State == "absent" {
		if _, err := ctx.FS.Stat(r.Dest); !os.IsNotExist(err) {
			return true, nil // Klasör var, silinmeli
		}
		return false, nil
	}

	// State == present
	if _, err := ctx.FS.Stat(r.Dest); os.IsNotExist(err) {
		return true, nil // Klasör yok, clone gerek
	}

	// Klasör var, burası bir git repo mu?
	if !r.isGitRepo(ctx, r.Dest) {
		return false, fmt.Errorf("directory %s exists but is not a git repository", r.Dest)
	}

	// Remote URL kontrolü (Güvenlik)
	currentRemote, err := getRemoteURL(ctx, r.Dest, r.Remote)
	if err != nil {
		return false, fmt.Errorf("failed to get remote url: %w", err)
	}
	// Basit eşleşme kontrolü (SSH vs HTTPS normalizasyonu yapmıyoruz, exact match bekliyoruz şimdilik)
	if currentRemote != r.Repo && !strings.Contains(currentRemote, r.Repo) {
		return false, fmt.Errorf("remote url mismatch: expected %s, got %s", r.Repo, currentRemote)
	}

	// Eğer spesifik bir commit isteniyorsa
	if r.Commit != "" {
		head, err := getHeadSHA(ctx, r.Dest)
		if err != nil {
			return false, err
		}
		if head != r.Commit {
			return true, nil // Commit farklı, checkout gerek
		}
	}

	// Eğer update isteniyorsa
	if r.Update {
		// Fetch yapıp durumu kontrol et
		if err := fetchRemote(ctx, r.Dest, r.Remote); err != nil {
			return false, fmt.Errorf("git fetch failed: %w", err)
		}

		// Branch takibi
		if r.Branch != "" {
			// Şu anki branch doğru mu?
			currentBranch, err := getCurrentBranch(ctx, r.Dest)
			if err != nil {
				return false, err
			}
			if currentBranch != r.Branch {
				return true, nil // Branch değiştirilmeli
			}

			// Remote ile fark var mı?
			start, err := getHeadSHA(ctx, r.Dest)
			if err != nil {
				return false, err
			}
			// Remote branch SHA
			remoteRef := fmt.Sprintf("%s/%s", r.Remote, r.Branch)
			remoteSHA, err := getRefSHA(ctx, r.Dest, remoteRef)
			if err != nil {
				return false, fmt.Errorf("remote ref not found: %s", remoteRef)
			}

			if start != remoteSHA {
				return true, nil // Update gerekli
			}
		}
	}

	return false, nil
}

func (r *GitAdapter) Apply(ctx *core.SystemContext) (core.Result, error) {
	needsAction, err := r.Check(ctx)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needsAction {
		return core.SuccessNoChange(fmt.Sprintf("Git repo %s is up to date", r.Repo)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Git %s %s to %s (Update: %v)", r.State, r.Repo, r.Dest, r.Update)), nil
	}

	if r.State == "absent" {
		if err := ctx.FS.RemoveAll(r.Dest); err != nil {
			return core.Failure(err, "Failed to remove directory"), err
		}
		return core.SuccessChange(fmt.Sprintf("Removed git repo at %s", r.Dest)), nil
	}

	// State == present

	// Create parent dir
	if err := ctx.FS.MkdirAll(r.Dest, 0755); err != nil && !os.IsExist(err) {
		return core.Failure(err, "Failed to create directory"), err
	}

	// Klasör yoksa Clone
	if _, err := ctx.FS.Stat(filepath.Join(r.Dest, ".git")); os.IsNotExist(err) {
		// Clone argümanları
		args := []string{"clone"}
		if r.Branch != "" {
			args = append(args, "-b", r.Branch)
		}
		args = append(args, "--", r.Repo, r.Dest)

		fullCmd := "git " + utils.ShellJoin(args...)
		out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
		if err != nil {
			return core.Failure(err, fmt.Sprintf("Git clone failed: %s", out)), err
		}

		r.IsNew = true // Yeni oluşturuldu

		// Eğer spesifik commit'e dönülecekse
		if r.Commit != "" {
			if err := checkout(ctx, r.Dest, r.Commit); err != nil {
				return core.Failure(err, "Git checkout commit failed"), err
			}
		}

		return core.SuccessChange(fmt.Sprintf("Cloned %s to %s", r.Repo, r.Dest)), nil
	}

	// Klasör varsa Update/Checkout

	// Rollback için current SHA'yı sakla
	currentSHA, _ := getHeadSHA(ctx, r.Dest)
	r.PreviousSHA = currentSHA

	// Fetch
	if r.Update || r.Commit != "" {
		if err := fetchRemote(ctx, r.Dest, r.Remote); err != nil {
			return core.Failure(err, "Git fetch failed"), err
		}
	}

	// Checkout Target (Commit > Tag > Branch)
	target := r.Branch
	if r.Tag != "" {
		target = r.Tag
	}
	if r.Commit != "" {
		target = r.Commit
	}

	if err := checkout(ctx, r.Dest, target); err != nil {
		return core.Failure(err, fmt.Sprintf("Git checkout %s failed", target)), err
	}

	// Eğer branch ise ve update isteniyorsa pull yap
	if r.Update && r.Commit == "" && r.Tag == "" {
		// git pull origin branch
		fullCmd := fmt.Sprintf("git -C %s pull %s %s", utils.ShellQuote(r.Dest), utils.ShellQuote(r.Remote), utils.ShellQuote(r.Branch))
		out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
		if err != nil {
			return core.Failure(err, fmt.Sprintf("Git pull failed: %s", out)), err
		}
		return core.SuccessChange("Updated git repo"), nil
	}

	return core.SuccessChange("Git repo updated/checked out"), nil
}

func (r *GitAdapter) Revert(ctx *core.SystemContext) error {
	// Yeni klonlandıysa sil
	if r.IsNew {
		pterm.Warning.Printf("Reverting git clone: removing %s\n", r.Dest)
		return ctx.FS.RemoveAll(r.Dest)
	}

	// Update edildiyse eski SHA'ya dön
	if r.PreviousSHA != "" {
		if err := checkout(ctx, r.Dest, r.PreviousSHA); err != nil {
			return fmt.Errorf("failed to revert git repo to %s: %w", r.PreviousSHA, err)
		}
	}

	return nil
}

// Helper Functions

func (r *GitAdapter) isGitRepo(ctx *core.SystemContext, path string) bool {
	_, err := ctx.FS.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func getRemoteURL(ctx *core.SystemContext, path, remote string) (string, error) {
	fullCmd := fmt.Sprintf("git -C %s remote get-url %s", utils.ShellQuote(path), utils.ShellQuote(remote))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func getHeadSHA(ctx *core.SystemContext, path string) (string, error) {
	fullCmd := fmt.Sprintf("git -C %s rev-parse HEAD", utils.ShellQuote(path))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func fetchRemote(ctx *core.SystemContext, path, remote string) error {
	fullCmd := fmt.Sprintf("git -C %s fetch %s", utils.ShellQuote(path), utils.ShellQuote(remote))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return fmt.Errorf("output: %s, error: %w", out, err)
	}
	return nil
}

func getCurrentBranch(ctx *core.SystemContext, path string) (string, error) {
	fullCmd := fmt.Sprintf("git -C %s rev-parse --abbrev-ref HEAD", utils.ShellQuote(path))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func getRefSHA(ctx *core.SystemContext, path, ref string) (string, error) {
	fullCmd := fmt.Sprintf("git -C %s rev-parse %s", utils.ShellQuote(path), utils.ShellQuote(ref))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func checkout(ctx *core.SystemContext, path, target string) error {
	fullCmd := fmt.Sprintf("git -C %s checkout %s", utils.ShellQuote(path), utils.ShellQuote(target))
	out, err := ctx.Transport.Execute(ctx.Context, fullCmd)
	if err != nil {
		return fmt.Errorf("output: %s, error: %w", out, err)
	}
	return nil
}
