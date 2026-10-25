package cmd

import (
	"fmt"
	"os"

	"atomicgo.dev/cursor"
	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var autoConfirm bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Veto system profile",
	Long:  fmt.Sprintf(`Scans the current system and creates a '%s' profile used for context-aware operations.`, consts.GetSystemProfilePath()),
	Run: func(cmd *cobra.Command, args []string) {
		runInit()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "Skip interactive prompts and save immediately")
}

func runInit() {
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgMagenta)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack, pterm.Bold)).
		Println("Veto System Initializer")

	spinner, _ := pterm.DefaultSpinner.Start("Scanning system...")
	// Init runs locally
	detectedCtx := core.NewSystemContext(false, transport.NewLocalTransport())
	system.Detect(detectedCtx)
	spinner.Success("System scan complete")
	pterm.Println()

	// Show Results
	displaySystemInfo(detectedCtx)

	if autoConfirm {
		pterm.Info.Println("Auto-confirm enabled. Saving profile...")
	} else {
		// Interaction Loop
		selection, _ := pterm.DefaultInteractiveSelect.
			WithOptions([]string{"Yes, looks good", "Customize", "Cancel"}).
			Show("Do you want to save this system profile?")

		if selection == "Cancel" {
			pterm.Info.Println("Initialization cancelled.")
			return
		}

		if selection == "Customize" {
			customizeProfile(detectedCtx)
			pterm.Println()
			pterm.Info.Println("Updated Profile:")
			displaySystemInfo(detectedCtx)
		}
	}

	// Save
	if err := saveSystemProfile(detectedCtx); err != nil {
		pterm.Error.Printf("Failed to save profile: %v\n", err)
		os.Exit(1)
	}

	pterm.Success.Printf("System profile saved to %s\n", consts.GetSystemProfilePath())

	// Offer to Import Resources
	pterm.Println()
	if !autoConfirm {
		result, _ := pterm.DefaultInteractiveConfirm.
			WithDefaultText("Would you like to scan and import existing system resources?").
			WithDefaultValue(true).
			Show()

		if result {
			RunImportInteractive("system.yaml", false)
		} else {
			pterm.Info.Println("Skipping import. You can run 'veto import' later.")
		}
	} else {
		// Auto-confirm enabled? Maybe specific flag for auto-import?
		// For now, let's just log info.
		pterm.Info.Println("Skipping auto-import (run 'veto import' manually or use interactive mode).")
	}

	pterm.Println()
	pterm.Info.Println("You can now run 'veto apply' to correct your system state!")
}

func displaySystemInfo(ctx *core.SystemContext) {
	data := [][]string{
		{"Kernel", ctx.Kernel},
		{"Distro", ctx.Distro},
		{"Version", ctx.Version},
		{"Init System", ctx.InitSystem},
		{"Hostname", ctx.Hostname},
		{"User", ctx.User},
		{"Home", ctx.HomeDir},
		{"Shell", ctx.Env.Shell},
		{"Language", ctx.Env.Lang},
		{"Timezone", ctx.Env.Timezone},
		{"CPU", fmt.Sprintf("%s (%d cores)", ctx.Hardware.CPUModel, ctx.Hardware.CPUCore)},
		{"RAM", ctx.Hardware.RAMTotal},
		{"GPU", ctx.Hardware.GPUVendor},
		{"FS", ctx.FSInfo.RootFSType},
	}
	pterm.DefaultTable.WithHasHeader(false).WithData(data).Render()
}

func customizeProfile(ctx *core.SystemContext) {
	cursor.Show()
	pterm.Info.Println("Enter new values (leave empty to keep current):")

	ctx.Distro = ask("Distro", ctx.Distro)
	ctx.Version = ask("Version", ctx.Version)
	ctx.Env.Shell = ask("Shell", ctx.Env.Shell)
	ctx.Env.Lang = ask("Language", ctx.Env.Lang)
	ctx.Env.Timezone = ask("Timezone", ctx.Env.Timezone)
	ctx.Hardware.GPUVendor = ask("GPU Vendor", ctx.Hardware.GPUVendor)

	cursor.Hide()
}

func ask(label, current string) string {
	prompt := fmt.Sprintf("%s [%s]: ", label, current)
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	if input == "" {
		return current
	}
	return input
}

func saveSystemProfile(ctx *core.SystemContext) error {
	// Create .veto dir if not exists
	vetoDir := consts.GetVetoDir()
	if err := os.MkdirAll(vetoDir, 0755); err != nil {
		return err
	}

	filePath := consts.GetSystemProfilePath()
	data, err := yaml.Marshal(ctx)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}
