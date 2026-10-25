package network

import (
	"fmt"
	"github.com/melih-ucgun/veto/internal/utils"
	"strconv"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

type UFWProvider struct{}

func (p *UFWProvider) CheckRule(ctx *core.SystemContext, r *FirewallAdapter) (bool, error) {
	// ufw status numbered output example:
	// [ 1] 22/tcp                   ALLOW IN    Anywhere
	// [ 2] 80                       ALLOW IN    Anywhere
	// ...

	// We simply check existence. Exact parsing is tricky.
	// For simplicity in V1:
	// If state=present: we assume we need to apply unless we are 100% sure it exists?
	// UFW allows duplicate rules sometimes, but usually "ufw allow X" is idempotent-ish (says Skipping adding existing rule).
	// So we can return true (needs action) freely if we rely on UFW's own idempotency.
	// BUT Veto prefers knowing ahead.

	// Let's implement basic grep-based check or just "always apply if present" because ufw handles it?
	// No, we want to know changed status.

	// Let's construct the "grep" string.
	// Rule: PORT/PROTO + ACTION + FROM
	// Ex: "80/tcp ALLOW Anywhere"
	// Or "80 ALLOW Anywhere" (if proto any)

	out, err := ctx.Transport.Execute(ctx.Context, "ufw status numbered")
	if err != nil {
		// If ufw not active/installed, err might happen.
		return false, fmt.Errorf("ufw status check failed: %w", err)
	}

	// Basic heuristic match
	portStr := strconv.Itoa(r.Port)
	if r.Proto != "any" {
		portStr += "/" + r.Proto
	}

	actionUpper := strings.ToUpper(r.Action)

	// Check if this appears in a line
	// This is not perfect exact matching but sufficient for MVP.
	// Improvements: full parsing.

	lines := strings.Split(out, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, portStr) && strings.Contains(line, actionUpper) {
			// Also check Source (From)
			if r.From == "any" {
				if strings.Contains(line, "Anywhere") {
					found = true
					break
				}
			} else {
				if strings.Contains(line, r.From) {
					found = true
					break
				}
			}
		}
	}

	if r.State == "absent" {
		return found, nil // If found, needs delete (Action=true)
	}

	return !found, nil // If not found, needs create (Action=true)
}

func (p *UFWProvider) ApplyRule(ctx *core.SystemContext, r *FirewallAdapter) (core.Result, error) {
	needs, err := p.CheckRule(ctx, r)
	if err != nil {
		return core.Failure(err, "Check failed"), err
	}
	if !needs {
		return core.SuccessNoChange(fmt.Sprintf("Firewall rule %d/%s is correct", r.Port, r.Proto)), nil
	}

	if ctx.DryRun {
		return core.SuccessChange(fmt.Sprintf("[DryRun] Firewall rule %d/%s state=%s", r.Port, r.Proto, r.State)), nil
	}

	// Construct command
	// ufw [delete] [action] [from X to any port Y proto Z]
	// Simpler syntax for basic port allow: "ufw allow 80/tcp"

	baseCmd := "ufw"
	if r.State == "absent" {
		baseCmd += " delete"
	}

	baseCmd += " " + utils.ShellQuote(r.Action)

	// If source is any and dest is any and simple port allow
	if r.From == "any" && r.To == "any" {
		portSpec := strconv.Itoa(r.Port)
		if r.Proto != "any" {
			portSpec += "/" + r.Proto
		}
		baseCmd += " " + utils.ShellQuote(portSpec)
	} else {
		// Complex syntax
		baseCmd += " from " + utils.ShellQuote(r.From) + " to " + utils.ShellQuote(r.To) + " port " + strconv.Itoa(r.Port)
		if r.Proto != "any" {
			baseCmd += " proto " + utils.ShellQuote(r.Proto)
		}
	}

	if out, err := ctx.Transport.Execute(ctx.Context, baseCmd); err != nil {
		return core.Failure(err, "UFW command failed: "+out), err
	}

	r.ActionPerformed = r.State // created or deleted ideally
	if r.State == "present" {
		return core.SuccessChange("Firewall rule added"), nil
	}
	return core.SuccessChange("Firewall rule deleted"), nil
}

func (p *UFWProvider) RevertRule(ctx *core.SystemContext, r *FirewallAdapter) error {
	// If we added it, delete it.
	// If we deleted it, add it back? (Hard without backup of strict previous rule)

	if r.State == "present" && r.ActionPerformed == "present" {
		// We added it. Delete it.
		// Construct delete command (same logic as Apply absent)

		// DRY violation here, but acceptable for now.
		// Should call ApplyRule with inverted state ideally?
		// But Apply checks adapter state.

		// Just run delete command best effort.
		cmd := fmt.Sprintf("ufw delete %s %d", utils.ShellQuote(r.Action), r.Port) // Simplified revert
		if r.Proto != "any" {
			cmd += "/" + r.Proto
		}

		_, err := ctx.Transport.Execute(ctx.Context, cmd)
		return err
	}
	return nil
}
