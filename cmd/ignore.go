package cmd

import (
	"fmt"
	"os"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/config"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/hub"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var ignoreCmd = &cobra.Command{
	Use:   "ignore [pattern]...",
	Short: fmt.Sprintf("Ignore resources using %s", consts.IgnoreFileName),
	Long:  fmt.Sprintf(`Add patterns to %s to exclude them from being tracked. Matches against resource names and paths.`, consts.IgnoreFileName),
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ignoreFile := consts.GetIgnoreFilePath()
		mgr, err := config.NewIgnoreManager(ignoreFile)
		if err != nil {
			pterm.Error.Printf("Failed to load ignore file: %v\n", err)
			return
		}

		// Load System Config to check for conflicts
		manager := hub.NewRecipeManager("")
		activeRecipe, _ := manager.GetActive()
		configPath := consts.GetSystemProfilePath()
		if activeRecipe != "" {
			path, err := manager.GetRecipePath(activeRecipe)
			if err == nil {
				configPath = path
			}
		}

		var currentConfig config.Config
		configLoaded := false
		if data, err := os.ReadFile(configPath); err == nil {
			if err := yaml.Unmarshal(data, &currentConfig); err == nil {
				configLoaded = true
			}
		}

		for _, pattern := range args {
			if err := mgr.Add(pattern); err != nil {
				pterm.Error.Printf("Failed to add '%s' to ignore list: %v\n", pattern, err)
				continue
			}
			pterm.Success.Printf("Added '%s' to %s\n", pattern, consts.IgnoreFileName)

			// Check conflict
			if configLoaded {
				var resourcesToRemove []int
				for i, res := range currentConfig.Resources {
					if mgr.IsIgnored(res.Name) { // Using the updated manager logic (IsIgnored checks full pattern match)
						// Match found!
						resourcesToRemove = append(resourcesToRemove, i)
					}
				}

				if len(resourcesToRemove) > 0 {
					pterm.Warning.Printf("Pattern '%s' matches %d existing resource(s) in your config.\n", pattern, len(resourcesToRemove))
					result, _ := pterm.DefaultInteractiveConfirm.
						WithDefaultText("Do you want to remove them from configuration?").
						WithDefaultValue(true).
						Show()

					if result {
						// Remove loop (reverse order to keep indices valid)
						// Just create a new slice for simplicity
						var newResources []config.ResourceConfig
						for i, res := range currentConfig.Resources {
							shouldRemove := false
							for _, idx := range resourcesToRemove {
								if i == idx {
									shouldRemove = true
									break
								}
							}
							if !shouldRemove {
								newResources = append(newResources, res)
							} else {
								pterm.Info.Printf("Removed: %s (%s)\n", res.Name, res.Type)
							}
						}
						currentConfig.Resources = newResources

						// Save
						data, _ := yaml.Marshal(currentConfig)
						if err := os.WriteFile(configPath, data, 0644); err != nil {
							pterm.Error.Printf("Failed to save configuration: %v\n", err)
						} else {
							pterm.Success.Println("Configuration updated.")
						}

						// Reload config to reflect changes for next pattern
						configLoaded = true // redundant but clear
					}
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(ignoreCmd)
}
