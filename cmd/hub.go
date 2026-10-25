package cmd

import (
	"os"
	"path/filepath"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/hub"
	"github.com/spf13/cobra"
)

var hubCmd = &cobra.Command{
	Use:   "hub",
	Short: "Manage recipes from the Veto Registry",
	Long:  `Search, update, and install recipes from the community registry.`,
}

var hubUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the local registry index",
	Run: func(cmd *cobra.Command, args []string) {
		client := hub.NewHubClient("")
		if err := client.Update(); err != nil {
			pterm.Error.Printf("Failed to update registry: %v\n", err)
			os.Exit(1)
		}
		pterm.Success.Println("Registry updated successfully")
	},
}

var hubSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for recipes",
	Run: func(cmd *cobra.Command, args []string) {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}

		client := hub.NewHubClient("")
		results, err := client.Search(query)
		if err != nil {
			pterm.Error.Printf("Search failed: %v\n", err)
			return
		}

		if len(results) == 0 {
			pterm.Info.Println("No recipes found matching your query.")
			return
		}

		pterm.DefaultSection.Printf("Found %d recipes:", len(results))
		for _, name := range results {
			pterm.Println(" - " + name)
		}
	},
}

var hubInstallCmd = &cobra.Command{
	Use:   "install [recipe_name] [as_name]",
	Short: "Install a recipe",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		recipeName := args[0]
		targetName := recipeName
		if len(args) > 1 {
			targetName = args[1]
		}

		// Determine target directory
		recipesDir, _ := consts.GetRecipesPath()
		targetDir := filepath.Join(recipesDir, targetName)

		pterm.Info.Printf("Installing '%s' to '%s'...\n", recipeName, targetName)

		pterm.Info.Printf("Installing '%s' to '%s'...\n", recipeName, targetName)

		if len(recipeName) > 6 && recipeName[:6] == "oci://" {
			// OCI Installation
			client := hub.NewOCIClient()
			if err := client.Pull(recipeName, targetDir); err != nil {
				pterm.Error.Printf("OCI Pull failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Standard Git Installation
			client := hub.NewHubClient("")
			if err := client.Install(recipeName, targetDir); err != nil {
				pterm.Error.Printf("Installation failed: %v\n", err)
				os.Exit(1)
			}
		}

		pterm.Success.Printf("Recipe installed! Use it with: veto apply %s\n", targetName)
	},
}

func init() {
	rootCmd.AddCommand(hubCmd)
	hubCmd.AddCommand(hubUpdateCmd)
	hubCmd.AddCommand(hubSearchCmd)
	hubCmd.AddCommand(hubInstallCmd)
}
