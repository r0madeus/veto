package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/config"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/resource"
	"github.com/r0madeus/veto/internal/state"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/spf13/cobra"
)

var checkMode bool
var detailedMode bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current state of managed resources",
	Long:  `Displays a list of resources managed by Veto. By default, it shows the last known state from history. Use --check to perform a live system audit (drift detection).`,
	Run: func(cmd *cobra.Command, args []string) {
		if !checkMode {
			showHistoryStatus()
		} else {
			runDriftCheck(cmd)
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&checkMode, "check", false, "Perform live drift check")
	statusCmd.Flags().BoolVarP(&detailedMode, "detailed", "d", false, "Show detailed diffs for drifted resources")
}

func showHistoryStatus() {
	statePath := consts.GetStateFilePath()
	// Status command typically runs locally, so we use RealFS
	mgr, err := state.NewManager(statePath, &core.RealFS{})
	if err != nil {
		pterm.Error.Printf("Could not load state file: %v\n", err)
		return
	}

	if len(mgr.Current.Resources) == 0 {
		pterm.Info.Println("No resources currently managed by Veto.")
		return
	}

	pterm.DefaultHeader.Println("Veto Status (Last Run)")
	pterm.Printf("Last Run: %s\n\n", mgr.Current.LastRun.Format(time.RFC822))

	tableData := [][]string{{"Type", "Name", "State", "Status", "Last Applied"}}

	for _, res := range mgr.Current.Resources {
		statusText := pterm.FgGreen.Sprint("Success")
		if res.Status != "success" {
			statusText = pterm.FgRed.Sprint(res.Status)
		}

		tableData = append(tableData, []string{
			res.Type,
			res.Name,
			res.State,
			statusText,
			res.LastApplied.Format("2006-01-02 15:04:05"),
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

func runDriftCheck(cmd *cobra.Command) {
	pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).Println("Veto Live Drift Check")
	spinner, _ := pterm.DefaultSpinner.Start("Loading configuration...")

	// 1. Setup Context
	ctx := core.NewSystemContext(false, transport.NewLocalTransport())
	system.Detect(ctx)

	// 2. Load Active Config
	// Use global config flag or default
	configFile, _ := cmd.Flags().GetString("config")
	if configFile == "" {
		configFile = "system.yaml"
	}

	cfg, err := config.LoadConfig(configFile, false) // Decrypt false? Maybe explicit?
	if err != nil {
		spinner.Fail(fmt.Sprintf("Failed to load config file '%s': %v", configFile, err))
		return
	}
	spinner.Success("Configuration loaded")

	// Set vars
	ctx.Vars = cfg.Vars

	// 3. Convert to ConfigItems
	// We verify everything (no dependency sorting needed strictly for check, but handy for order)
	// Just simple conversion
	var items []core.ConfigItem
	for _, res := range cfg.Resources {
		items = append(items, core.ConfigItem{
			Name:   res.Name,
			Type:   res.Type,
			State:  res.State,
			When:   res.When,
			Params: res.Params,
		})
	}

	spinner.UpdateText("Auditing system state...")
	results, err := core.CheckDrift(items, resource.CreateResourceWithParams, ctx)
	if err != nil {
		spinner.Fail(fmt.Sprintf("Audit failed: %v", err))
		return
	}
	spinner.Success("Audit complete")
	pterm.Println()

	// 4. Render Table
	tableData := [][]string{{"Type", "Name", "Desired", "Live Status", "Details"}}
	driftCount := 0

	for _, res := range results {
		statusText := ""
		switch res.Status {
		case core.StatusSynced:
			statusText = pterm.FgGreen.Sprint("✅ Synced")
		case core.StatusDrifted:
			statusText = pterm.FgRed.Sprint("❌ Drifted")
			driftCount++
		case core.StatusError:
			statusText = pterm.FgRed.Sprint("⚠️ Error")
			driftCount++
		default:
			statusText = pterm.FgYellow.Sprint("Unknown")
		}

		detail := res.Detail
		if detailedMode && res.Diff != "" {
			detail = res.Detail + "\n(Run with --detailed for full diff)"
		}

		tableData = append(tableData, []string{
			res.Type,
			res.Name,
			res.Desired,
			statusText,
			detail,
		})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
	pterm.Println()

	if driftCount > 0 {
		pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgRed, pterm.Bold)).
			Printf("System is DRIFTED! %d resources out of sync.\n", driftCount)
		pterm.Info.Println("Run 'veto apply' to correct these issues.")
		os.Exit(1) // Exit with error for CI/CD
	} else {
		pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgGreen, pterm.Bold)).
			Println("System is completely IN SYNC.")
	}
}
