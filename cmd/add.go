package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/config"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/hub"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var forceType string
var asService bool

// ActionItem represents a single step performed during the add operation
type ActionItem struct {
	Icon   string
	Action string
	Detail string
}

var addCmd = &cobra.Command{
	Use:   "add [resource_name/path]...",
	Short: "Add a new resource to configuration",
	Long:  `Intelligently adds resources to the active configuration profile. Detects files, packages, and services automatically.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		manager := hub.NewRecipeManager("")
		activeRecipe, _ := manager.GetActive()

		configPath := consts.GetSystemProfilePath()
		if activeRecipe != "" {
			path, err := manager.GetRecipePath(activeRecipe)
			if err == nil {
				configPath = path
			}
		}

		// Verify config exists
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			pterm.Error.Printf("Config file '%s' not found. Run 'veto init' first.\n", configPath)
			return
		}

		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgMagenta)).Println("Veto Resource Add")
		pterm.Info.Printf("Target Config: %s\n", configPath)

		// Only local context needed for adding user
		ctx := core.NewSystemContext(false, transport.NewLocalTransport())
		system.Detect(ctx)

		for _, arg := range args {
			pterm.Println()
			pterm.DefaultSection.Printf("Processing: %s", arg)

			res := detectResource(arg, ctx)
			if res == nil {
				continue
			}

			// Check Ignore List
			ignoreMgr, _ := config.NewIgnoreManager(consts.GetIgnoreFilePath())
			if ignoreMgr != nil && ignoreMgr.IsIgnored(res.Name) {
				pterm.Warning.Printf("Resource '%s' is ignored by %s. Skipping.\n", res.Name, consts.IgnoreFileName)
				continue
			}

			actions, err := appendResourceToConfig(configPath, *res)
			if err != nil {
				pterm.Error.Printf("Failed to add '%s': %v\n", arg, err)
				continue
			}

			// Report Actions
			if len(actions) > 0 {
				pterm.Println(pterm.FgGray.Sprint("Actions Taken:"))
				for i, act := range actions {
					pterm.Printf(" %d. %s %s: %s\n", i+1, act.Icon, pterm.Bold.Sprint(act.Action), act.Detail)
				}
			}

			pterm.Println()
			pterm.Success.Printf("✨ Resource '%s' added successfully!\n", res.Name)
		}
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&forceType, "type", "t", "", "Force resource type (pkg, file, service)")
	addCmd.Flags().BoolVarP(&asService, "service", "s", false, "Treat as service")
}

func detectResource(input string, ctx *core.SystemContext) *config.ResourceConfig {
	// 1. Force Type
	if forceType != "" {
		return &config.ResourceConfig{
			Type:  forceType,
			Name:  input,
			State: "present",
			Params: map[string]interface{}{
				"path": input, // Just in case it's a file
			},
		}
	}

	// 2. Service Flag
	if asService {
		return &config.ResourceConfig{
			Type:  "service",
			Name:  input,
			State: "running",
			Params: map[string]interface{}{
				"enabled": true,
			},
		}
	}

	// 3. Smart Detection

	// A. File Detection
	// Expand tilde
	expanded := input
	if strings.HasPrefix(input, "~/") {
		home, _ := os.UserHomeDir()
		expanded = filepath.Join(home, input[2:])
	}

	if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
		// It is a file!
		absPath, _ := filepath.Abs(expanded)
		return &config.ResourceConfig{
			Type: "file",
			Name: filepath.Base(input),
			// Name collision risk managed by user for now
			State: "present",
			Params: map[string]interface{}{
				"path": absPath,
			},
		}
	}

	// B. Package Detection
	// Check simple heurustic
	if !strings.Contains(input, "/") && !strings.Contains(input, "\\") {
		// Assume package
		return &config.ResourceConfig{
			Type:  "pkg",
			Name:  input,
			State: "present",
		}
	}

	pterm.Warning.Printf("Could not detect type for '%s'. Use --type flag.\n", input)
	return nil
}

func appendResourceToConfig(path string, res config.ResourceConfig) ([]ActionItem, error) {
	var actions []ActionItem

	// If resource is a FILE, perform Move & Symlink logic
	if res.Type == "file" {
		targetPath := res.Params["path"].(string) // Original symlink target
		absTarget, _ := filepath.Abs(targetPath)

		// Calculate destination in .veto/files/
		homeDir, _ := os.UserHomeDir()

		var storageRelPath string
		if strings.HasPrefix(absTarget, homeDir) {
			rel, _ := filepath.Rel(homeDir, absTarget)
			storageRelPath = filepath.Join(consts.FilesDirName, rel) // .veto/files/...
		} else {
			// Outside home? Use full path as structure
			storageRelPath = filepath.Join(consts.FilesDirName, "root", absTarget)
		}

		// .veto directory root (where system.yaml is)
		vetoRoot := filepath.Dir(path)
		storageAbsPath := filepath.Join(vetoRoot, storageRelPath)

		// 1. Create directory structure
		if err := os.MkdirAll(filepath.Dir(storageAbsPath), 0755); err != nil {
			return actions, fmt.Errorf("failed to create storage dir: %w", err)
		}

		// 2. Move File (if it's not already there)
		// Check if source is already a symlink pointing to our storage?
		info, err := os.Lstat(absTarget)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				linkDest, _ := os.Readlink(absTarget)
				if linkDest == storageAbsPath {
					actions = append(actions, ActionItem{"ℹ️", "Link Exists", "File is already linked to storage"})
				} else {
					actions = append(actions, ActionItem{"⚠️", "Conflict", fmt.Sprintf("Replacing existing link -> %s", linkDest)})
					// Remove old link
					os.Remove(absTarget)
					// Create new link (will happen below)
				}
			} else {
				// Regular file: Move it.
				if err := moveFile(absTarget, storageAbsPath); err != nil {
					return actions, fmt.Errorf("failed to move file to storage: %w", err)
				}
				actions = append(actions, ActionItem{"🚚", "File Moved", fmt.Sprintf("%s -> %s", absTarget, storageRelPath)})

				// 3. Create Symlink
				if err := os.Symlink(storageAbsPath, absTarget); err != nil {
					// Rolling back move?
					_ = moveFile(storageAbsPath, absTarget)
					return actions, fmt.Errorf("failed to create symlink: %w", err)
				}
				actions = append(actions, ActionItem{"🔗", "Link Created", fmt.Sprintf("%s -> %s", absTarget, storageRelPath)})
			}
		}

		// Update Resource Params
		res.Params["source"] = storageRelPath // Relative to system.yaml
		res.Params["method"] = "symlink"

		// Sanitize Target Path: Replace /home/user with ${HOME}
		sanitizedTarget := absTarget
		if strings.HasPrefix(absTarget, homeDir) {
			sanitizedTarget = strings.Replace(absTarget, homeDir, "${HOME}", 1)
		}
		res.Params["path"] = sanitizedTarget
	}

	// Read existing
	data, err := os.ReadFile(path)
	if err != nil {
		return actions, err
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return actions, err
	}

	// Check duplicate
	for _, r := range cfg.Resources {
		if r.Type == res.Type && (r.Name == res.Name || (r.Params["path"] == res.Params["path"])) {
			return actions, fmt.Errorf("resource already exists")
		}
	}

	// Append
	cfg.Resources = append(cfg.Resources, res)

	// Write back
	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return actions, err
	}

	if err := os.WriteFile(path, newData, 0644); err != nil {
		return actions, err
	}

	actions = append(actions, ActionItem{"📝", "Config Updated", fmt.Sprintf("Added '%s:%s' to %s", res.Type, res.Name, filepath.Base(path))})

	return actions, nil
}

// moveFile attempts to move a file using os.Rename, falling back to Copy+Delete for cross-device moves
func moveFile(source, dest string) error {
	err := os.Rename(source, dest)
	if err == nil {
		return nil
	}

	// Check for cross-device link error
	if strings.Contains(err.Error(), "cross-device link") || strings.Contains(err.Error(), "EXDEV") {
		// Fallback to Copy + Delete
		if err := copyFile(source, dest); err != nil {
			return err
		}
		return os.Remove(source)
	}

	return err
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	// Copy permissions? For now simple write.
	// Ideally we stat to get mode.
	info, err := os.Stat(src)
	if err != nil {
		return os.WriteFile(dst, input, 0644)
	}
	return os.WriteFile(dst, input, info.Mode())
}
