package identity

import (
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strconv"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

type LinuxIdentityProvider struct{}

func (p *LinuxIdentityProvider) CheckUser(ctx *core.SystemContext, r *UserAdapter) (bool, error) {
	// getent passwd <user>
	out, err := ctx.Transport.Execute(ctx.Context, "getent passwd "+utils.ShellQuote(r.Name))
	exists := (err == nil)

	if r.State == "absent" {
		return exists, nil
	}

	if !exists {
		return true, nil // Needs to be created
	}

	// Parse output
	parts := strings.Split(strings.TrimSpace(out), ":")
	if len(parts) < 7 {
		return false, fmt.Errorf("unexpected output from getent: %s", out)
	}

	currentUid := parts[2]
	currentGid := parts[3]
	currentHome := parts[5]
	currentShell := parts[6]

	if r.Uid != "" && r.Uid != currentUid {
		return true, nil
	}
	if r.Gid != "" && r.Gid != currentGid {
		return true, nil
	}
	if r.Home != "" && r.Home != currentHome {
		return true, nil
	}
	if r.Shell != "" && r.Shell != currentShell {
		return true, nil
	}

	if len(r.Groups) > 0 {
		outGroups, err := ctx.Transport.Execute(ctx.Context, "id -Gn "+utils.ShellQuote(r.Name))
		if err != nil {
			return false, fmt.Errorf("failed to get user groups: %w", err)
		}
		currentGroups := strings.Fields(outGroups)
		groupMap := make(map[string]bool)
		for _, g := range currentGroups {
			groupMap[g] = true
		}
		for _, g := range r.Groups {
			if !groupMap[g] {
				return true, nil
			}
		}
	}

	return false, nil
}

func (p *LinuxIdentityProvider) ApplyUser(ctx *core.SystemContext, r *UserAdapter) (core.Result, error) {
	// Re-check action needs
	needs, err := p.CheckUser(ctx, r)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needs {
		return core.SuccessNoChange(fmt.Sprintf("User %s is correct", r.Name)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] User %s state=%s", r.Name, r.State)), nil
	}

	if r.State == "absent" {
		fullCmd := "userdel -r " + utils.ShellQuote(r.Name)
		if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
			return core.Failure(err, "Failed to delete user: "+out), err
		}
		r.ActionPerformed = "deleted"
		return core.SuccessChange("User deleted"), nil
	}

	exists := false
	if _, err := ctx.Transport.Execute(ctx.Context, "id -u "+utils.ShellQuote(r.Name)); err == nil {
		exists = true
	}

	cmd := "useradd"
	if exists {
		cmd = "usermod"
	}

	args := []string{}
	if r.Uid != "" {
		args = append(args, "-u", r.Uid)
	}
	if r.Gid != "" {
		args = append(args, "-g", r.Gid)
	}
	if r.Home != "" {
		args = append(args, "-d", r.Home, "-m")
	}
	if r.Shell != "" {
		args = append(args, "-s", r.Shell)
	}
	if len(r.Groups) > 0 {
		args = append(args, "-G", strings.Join(r.Groups, ","))
	}
	if !exists && r.System {
		args = append(args, "-r")
	}

	args = append(args, r.Name)

	fullCmd := cmd + " " + utils.ShellJoin(args...)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return core.Failure(err, fmt.Sprintf("Failed to %s user: %s", cmd, out)), err
	}

	if exists {
		r.ActionPerformed = "modified"
		return core.SuccessChange("User modified"), nil
	}

	r.ActionPerformed = "created"
	return core.SuccessChange("User created"), nil
}

func (p *LinuxIdentityProvider) RevertUser(ctx *core.SystemContext, r *UserAdapter) error {
	if r.ActionPerformed == "created" {
		fullCmd := "userdel -r " + utils.ShellQuote(r.Name)
		if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
			return fmt.Errorf("failed to revert user creation: %s: %w", out, err)
		}
	}
	return nil
}

func (p *LinuxIdentityProvider) CheckGroup(ctx *core.SystemContext, r *GroupAdapter) (bool, error) {
	out, err := ctx.Transport.Execute(ctx.Context, "getent group "+utils.ShellQuote(r.Name))
	exists := (err == nil)

	if r.State == "absent" {
		return exists, nil
	}

	if !exists {
		return true, nil
	}

	if r.Gid != -1 {
		parts := strings.Split(strings.TrimSpace(out), ":")
		if len(parts) >= 3 {
			currentGid, err := strconv.Atoi(parts[2])
			if err == nil && currentGid != r.Gid {
				return true, nil
			}
		}
	}

	return false, nil
}

func (p *LinuxIdentityProvider) ApplyGroup(ctx *core.SystemContext, r *GroupAdapter) (core.Result, error) {
	needs, err := p.CheckGroup(ctx, r)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needs {
		return core.SuccessNoChange(fmt.Sprintf("Group %s is correct", r.Name)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Group %s state=%s", r.Name, r.State)), nil
	}

	if r.State == "absent" {
		fullCmd := "groupdel " + utils.ShellQuote(r.Name)
		if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
			return core.Failure(err, "Failed to delete group: "+out), err
		}
		r.ActionPerformed = "deleted"
		return core.SuccessChange("Group deleted"), nil
	}

	// Modify?
	if _, err := ctx.Transport.Execute(ctx.Context, "getent group "+utils.ShellQuote(r.Name)); err == nil {
		args := []string{}
		if r.Gid != -1 {
			args = append(args, "-g", strconv.Itoa(r.Gid))
		}
		args = append(args, r.Name)

		if len(args) > 1 {
			fullCmd := "groupmod " + utils.ShellJoin(args...)
			if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
				return core.Failure(err, "Failed to modify group: "+out), err
			}
			r.ActionPerformed = "modified"
			return core.SuccessChange("Group modified"), nil
		}
		return core.SuccessNoChange("Group exists"), nil
	}

	// Create
	args := []string{}
	if r.Gid != -1 {
		args = append(args, "-g", strconv.Itoa(r.Gid))
	}
	if r.System {
		args = append(args, "-r")
	}
	args = append(args, r.Name)

	fullCmd := "groupadd " + utils.ShellJoin(args...)
	if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
		return core.Failure(err, "Failed to create group: "+out), err
	}
	r.ActionPerformed = "created"

	return core.SuccessChange("Group created"), nil
}

func (p *LinuxIdentityProvider) RevertGroup(ctx *core.SystemContext, r *GroupAdapter) error {
	if r.ActionPerformed == "created" {
		fullCmd := "groupdel " + utils.ShellQuote(r.Name)
		if out, err := ctx.Transport.Execute(ctx.Context, fullCmd); err != nil {
			return fmt.Errorf("failed to revert group creation: %s: %w", out, err)
		}
	}
	return nil
}
