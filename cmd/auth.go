package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/inventory"
)

// ensureSudoPasswords checks if any host needs a sudo password and prompts the user if missing.
func ensureSudoPasswords(hosts []inventory.Host) error {
	// Group hosts by user to minimize prompts
	// Map: User -> Password
	userPasswords := make(map[string]string)

	for i := range hosts {
		h := &hosts[i]

		// Determine effective user
		user := h.User
		if user == "" {
			user = "root" // Default assumption? Or current user?
			// transport defaults to current user if empty usually, but let's say "current"
		}

		// Check if sudo is needed
		// How do we know if sudo is needed per host?
		// Inventory vars? "ansible_become": "true"
		// or "ansible_become_method": "sudo"
		// For now Veto doesn't have strict "become" flag in inventory struct, only Vars.

		becomeMethod := h.Vars["ansible_become_method"]
		if becomeMethod == "" {
			// specific logic: if user is not root, maybe we need it?
			// But Veto philosophy: explicit.
			// Let's rely on "ansible_become_method" == "sudo"
			// OR if the user ran with global --sudopass? (future)
			continue
		}

		if becomeMethod == "sudo" {
			// Check if password exists
			if _, ok := h.Vars["ansible_become_password"]; !ok {
				// Check if we already have it for this user
				if pass, known := userPasswords[user]; known {
					h.Vars["ansible_become_password"] = pass
				} else {
					// Prompt
					pterm.Print(fmt.Sprintf("\n🔐 Sudo password for %s@%s: ", user, h.Name))
					pass, err := pterm.DefaultInteractiveTextInput.WithMask("*").Show()
					if err != nil {
						return fmt.Errorf("failed to read password: %w", err)
					}
					// Trim space?
					pass = strings.TrimSpace(pass) // Usually safe for typed passwords.

					// Cache it
					userPasswords[user] = pass
					h.Vars["ansible_become_password"] = pass
				}
			}
		}
	}
	return nil
}
