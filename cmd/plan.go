package cmd

import (
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/config"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/resource"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [config_file]",
	Short: "Preview changes without applying them",
	Long:  `Calculates the difference between the desired state (config) and the current system state.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		if len(args) > 0 {
			configPath = args[0]
		}

		// 1. System Detection
		pterm.DefaultHeader.Println("Veto Plan: Dry Run")
		spinner, _ := pterm.DefaultSpinner.Start("Loading configuration & context...")

		// Force DryRun in Context?
		// Engine.Plan doesn't modify anything, but context might affect templates.
		// We use standard context.
		// Plan works locally to generate plan for local? Or can use inventory?
		// Existing code assumes local.
		ctx := core.NewSystemContext(false, transport.NewLocalTransport())
		system.Detect(ctx)
		ctx.DryRun = true // explicit

		// 2. Load Config
		decrypt, _ := cmd.Flags().GetBool("decrypt")
		cfg, err := config.LoadConfig(configPath, decrypt)
		if err != nil {
			spinner.Fail("Failed to load config: " + err.Error())
			os.Exit(1)
		}
		spinner.Success("Configuration loaded")

		// 3. Sort Resources (Dependency Graph)
		// We flatten layers for a simple sequential plan or keep layers?
		// Plan output is usually a flat list of "what will happen".
		// Engine.Plan takes []ConfigItem. We can pass all items in topological order.

		// 3. Sort Resources (Dependency Graph)
		layers, err := config.SortResources(cfg.Resources)
		if err != nil {
			pterm.Error.Println("Dependency sorting failed:", err)
			os.Exit(1)
		}

		// Flatten layers into a single list for Plan (checking order matters for variables/templates but Plan is read-only)
		var allItems []core.ConfigItem
		for _, layer := range layers {
			for _, res := range layer {
				item := core.ConfigItem{
					Name:   res.Name,
					Type:   res.Type,
					State:  res.State,
					When:   res.When,
					Params: res.Params,
				}

				// Fix: Resolve Name if empty (logic copied from apply.go)
				if item.Name == "" {
					if n, ok := res.Params["name"].(string); ok {
						item.Name = n
					}
				}
				if item.Name == "" {
					item.Name = res.ID
				}

				allItems = append(allItems, item)
			}
		}

		// 4. Execute Plan
		eng := core.NewEngine(ctx, nil) // No state updater needed for plan

		spinner.UpdateText("Calculating plan...")
		planResult, err := eng.Plan(allItems, resource.CreateResourceWithParams)
		if err != nil {
			spinner.Fail("Planning failed: " + err.Error())
			os.Exit(1)
		}
		spinner.Success("Plan calculated")
		pterm.Println()

		// 5. Render Output
		if len(planResult.Changes) == 0 {
			pterm.Info.Println("No changes detected. System is in sync.")
			return
		}

		pterm.Println(pterm.FgCyan.Sprint("The following changes will be made:"))
		pterm.Println()

		for _, change := range planResult.Changes {
			switch change.Action {
			case "apply":
				pterm.Printf("  %s %s \"%s\" %s\n",
					pterm.FgGreen.Sprint("+"),
					pterm.Bold.Sprint(change.Type),
					change.Name,
					pterm.FgGreen.Sprint("will be applied"))

				// Display Diff if available
				if change.Diff != "" {
					lines := strings.Split(change.Diff, "\n")
					pterm.Println("    " + pterm.FgGray.Sprint("┌──────────────────────────────────────────┐"))
					for _, line := range lines {
						if line == "" {
							continue
						}

						// Colorize based on first character
						var out string
						if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
							out = pterm.FgGreen.Sprint(line)
						} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
							out = pterm.FgRed.Sprint(line)
						} else if strings.HasPrefix(line, "@@") {
							out = pterm.FgCyan.Sprint(line)
						} else if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
							out = pterm.Bold.Sprint(line)
						} else {
							out = line
						}
						pterm.Printf("    "+pterm.FgGray.Sprint("│ ")+"%s\n", out)
					}
					pterm.Println("    " + pterm.FgGray.Sprint("└──────────────────────────────────────────┘"))
				}
			case "noop":
				// Usually hidden
			}
		}

		pterm.Println()
		pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgCyan, pterm.Bold)).
			Printf("Summary: %d resource(s) to be updated.\n", len(planResult.Changes))
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
