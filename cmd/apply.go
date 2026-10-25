package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/melih-ucgun/veto/internal/adapters/snapshot"
	"github.com/melih-ucgun/veto/internal/config"
	"github.com/melih-ucgun/veto/internal/consts"
	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/fleet" // New import
	"github.com/melih-ucgun/veto/internal/hub"
	"github.com/melih-ucgun/veto/internal/resource"
	"github.com/melih-ucgun/veto/internal/state"
	"github.com/melih-ucgun/veto/internal/system"
	"github.com/melih-ucgun/veto/internal/transport"
)

var dryRun bool
var noSnapshot bool
var pruneMode bool
var inventoryFile string
var concurrency int

var applyCmd = &cobra.Command{
	Use:   "apply [config_file]",
	Short: "Apply the configuration to the system",
	Long: fmt.Sprintf(`Reads the configuration file and ensures system state matches desired state.
Updates %s with the results.`, consts.GetStateFilePath()),
	Run: func(cmd *cobra.Command, args []string) {
		var configFile string
		if len(args) > 0 {
			configFile = args[0]
		} else {
			// Check active recipe
			mgr := hub.NewRecipeManager("")
			recipePath, err := mgr.GetRecipePath("")
			if err == nil && recipePath != "" {
				// Verify file exists
				if _, err := os.Stat(recipePath); err == nil {
					configFile = recipePath
					pterm.Info.Printf("Using active recipe: %s\n", recipePath)
				}
			}

			// Fallback
			if configFile == "" {
				configFile = "system.yaml"
				pterm.Warning.Println("No active recipe found. Defaulting to system.yaml")
			}
		}

		decrypt, _ := cmd.Flags().GetBool("decrypt")
		if err := runApply(configFile, inventoryFile, concurrency, dryRun, noSnapshot, pruneMode, decrypt); err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Simulate changes without applying them")
	applyCmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "Disable automatic BTRFS snapshots")
	applyCmd.Flags().BoolVar(&pruneMode, "prune", false, "Remove unmanaged package resources (DESTRUCTIVE)")
	applyCmd.Flags().StringVarP(&inventoryFile, "inventory", "i", "", "Path to inventory file")
	applyCmd.Flags().IntVarP(&concurrency, "concurrency", "C", 5, "Number of concurrent hosts")
}

func runApply(configFile, invFile string, concurrency int, isDryRun bool, skipSnapshot bool, isPrune bool, decrypt bool) error {
	// Header
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack, pterm.Bold)).
		Println("Veto Config Manager")

	if isDryRun {
		pterm.ThemeDefault.SecondaryStyle.Println("Running in DRY-RUN mode")
	}

	// 1. Detect System (Local Context for Info/Snapshots)
	localTransport := transport.NewLocalTransport()
	ctx := core.NewSystemContext(isDryRun, localTransport)

	// Set Logger Level
	logLevel := core.LevelInfo
	switch verboseCount {
	case 1:
		logLevel = core.LevelInfo
	case 2:
		logLevel = core.LevelDebug
	case 3:
		logLevel = core.LevelTrace
	}
	ctx.Logger.SetLevel(logLevel)
	localTransport.Logger = ctx.Logger

	system.Detect(ctx)

	// 1.5 Load System Profile (if exists)
	if data, err := os.ReadFile(consts.GetSystemProfilePath()); err == nil {
		pterm.Info.Printf("Loading system profile from %s\n", consts.GetSystemProfilePath())
		// Override detected context with saved profile
		if err := yaml.Unmarshal(data, ctx); err != nil {
			pterm.Warning.Printf("Failed to parse system profile: %v\n", err)
		}
	}

	// Snapshot Manager Setup
	var snapMgr *snapshot.Manager
	var preSnapID string

	// Check if we should attempt a snapshot
	if !isDryRun && !skipSnapshot {
		// NewManager akıllıca seçim yapar (Snapper > Timeshift)
		snapMgr = snapshot.NewManager(ctx)

		if snapMgr != nil && snapMgr.IsAvailable(ctx) {
			pterm.Info.Printf("Snapshot System: %s detected\n", snapMgr.ProviderName())

			id, err := snapMgr.CreatePreSnapshot(ctx, fmt.Sprintf("Pre-Veto Apply: %s", configFile))
			if err != nil {
				pterm.Warning.Printf("Snapshot failed: %v (continuing anyway)\n", err)
			} else {
				preSnapID = id
				// Timeshift 'done' döndürür, Snapper ID döndürür.
				if id != "done" {
					pterm.Success.Printf("Pre-snapshot created: #%s\n", id)
				}
			}
		}
	}

	// System Info Box
	sysInfo := [][]string{
		{"OS", fmt.Sprintf("%s (%s)", ctx.Distro, ctx.Version)},
		{"Kernel", ctx.OS},
		{"Host", ctx.Hostname},
		{"User", fmt.Sprintf("%s (uid=%s)", ctx.User, ctx.UID)},
		{"CPU", ctx.Hardware.CPUModel},
		{"Cores", fmt.Sprintf("%d", ctx.Hardware.CPUCore)},
		{"RAM", ctx.Hardware.RAMTotal},
		{"GPU", fmt.Sprintf("%s %s", ctx.Hardware.GPUVendor, ctx.Hardware.GPUModel)},
		{"Shell", ctx.Env.Shell},
		{"FS", ctx.FSInfo.RootFSType},
		{"Time", time.Now().Format("15:04:05")},
	}
	pterm.DefaultTable.WithHasHeader(false).WithData(sysInfo).Render()
	pterm.Println()

	// 2. Initialize State Manager
	statePath := consts.GetStateFilePath()
	// Use ctx.Transport.GetFileSystem() for remote state capability
	stateMgr, err := state.NewManager(statePath, ctx.Transport.GetFileSystem())
	if err != nil {
		pterm.Warning.Printf("Could not initialize state manager: %v\n", err)
	}

	// 3. Load Configuration
	spinnerLoad, _ := pterm.DefaultSpinner.Start("Loading configuration...")
	cfg, err := config.LoadConfig(configFile, decrypt)
	if err != nil {
		spinnerLoad.Fail(fmt.Sprintf("Error loading config file '%s': %v", configFile, err))
		return err
	}
	spinnerLoad.Success("Configuration loaded")
	ctx.Vars = cfg.Vars

	// 4. Sort Resources (Global for both Local and Fleet)
	spinnerSort, _ := pterm.DefaultSpinner.Start("Resolving dependencies...")
	sortedResources, err := config.SortResources(cfg.Resources)
	if err != nil {
		spinnerSort.Fail(fmt.Sprintf("Error sorting resources: %v", err))
		return err
	}
	spinnerSort.Success(fmt.Sprintf("Resolved %d layers", len(sortedResources)))
	pterm.Println()

	// 4.1 Convert Config Resources to Core ConfigItems (Layers)
	// This is needed for both Local (optional refactor) and Fleet (required).
	var layers [][]core.ConfigItem
	for _, layer := range sortedResources {
		var layerItems []core.ConfigItem
		for _, res := range layer {
			name := res.Name
			if name == "" {
				if n, ok := res.Params["name"].(string); ok {
					name = n
				}
			}
			if name == "" {
				name = res.ID
			}
			state := res.State
			if state == "" {
				if s, ok := res.Params["state"].(string); ok {
					state = s
				}
			}
			layerItems = append(layerItems, core.ConfigItem{
				Name:   name,
				Type:   res.Type,
				State:  state,
				When:   res.When,
				Params: res.Params,
				Hooks: core.Hooks{
					Pre:      res.Hooks.Pre,
					Post:     res.Hooks.Post,
					OnChange: res.Hooks.OnChange,
					OnFail:   res.Hooks.OnFail,
				},
				Prune: res.Prune,
			})
		}
		layers = append(layers, layerItems)
	}

	// 5. Define Resource Creator
	createFn := func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
		return resource.CreateResourceWithParams(t, n, p, c)
	}

	// 6. Branch: Fleet vs Local
	if invFile != "" {
		// --- FLEET MODE ---
		pterm.DefaultHeader.WithFullWidth().
			WithBackgroundStyle(pterm.NewStyle(pterm.BgMagenta)).
			WithTextStyle(pterm.NewStyle(pterm.FgWhite, pterm.Bold)).
			Println("FLEET MODE ACTIVATED")

		inv, err := loadInventory(invFile)
		if err != nil {
			return fmt.Errorf("failed to load inventory: %w", err)
		}

		if err := ensureSudoPasswords(inv.Hosts); err != nil {
			return err
		}

		fleetMgr := fleet.NewFleetManager(inv.Hosts, isDryRun, isPrune, ctx.Logger)
		if err := fleetMgr.ApplyConfig(layers, concurrency, createFn); err != nil {
			return err
		}
		return nil
	}

	// --- LOCAL MODE (Remaining logic) ---
	// 5. Prepare Engine (For Local)
	pterm.DefaultHeader.WithFullWidth().
		WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).
		WithTextStyle(pterm.NewStyle(pterm.FgWhite, pterm.Bold)).
		Println("LOCAL MODE ACTIVATED")

	eng := core.NewEngine(ctx, stateMgr)
	finalError := error(nil)

	// Execute Layers
	for i, layer := range layers {
		// Layer Header
		pterm.DefaultSection.Printf("Phase %d: Processing %d resources", i+1, len(layer))

		// Spinner for execution (Simple main spinner)
		spinnerExec, _ := pterm.DefaultSpinner.Start("Executing layer...")

		if err := eng.RunParallel(layer, createFn); err != nil {
			spinnerExec.Fail(fmt.Sprintf("Layer %d failed", i+1))
			pterm.Error.Printf("Layer %d completed with errors: %v\n", i+1, err)
			finalError = err // Keep track of error
			break            // Stop processing layers on failure
		}
		spinnerExec.Success(fmt.Sprintf("Layer %d complete", i+1))
	}

	// Post Snapshot Logic
	if preSnapID != "" && snapMgr != nil {
		desc := fmt.Sprintf("Post-Veto Apply: %s", configFile)
		if finalError != nil {
			desc += " (Failed)"
		}

		// Manager arka tarafta Timeshift ise post-snapshot'ı atlayabilir
		if err := snapMgr.CreatePostSnapshot(ctx, preSnapID, desc); err != nil {
			pterm.Warning.Printf("Post-snapshot failed: %v\n", err)
		}
	}

	if finalError != nil {
		return finalError
	}

	// 7. Prune Logic (Strict Mode)
	if isPrune {
		pterm.Println()
		pterm.DefaultHeader.WithFullWidth().
			WithBackgroundStyle(pterm.NewStyle(pterm.BgRed)).
			WithTextStyle(pterm.NewStyle(pterm.FgWhite, pterm.Bold)).
			Println("PRUNE MODE (Destructive)")

		// Convert all resources to ConfigItems for Prune
		var allItems []core.ConfigItem
		for _, res := range cfg.Resources {
			allItems = append(allItems, core.ConfigItem{
				Name:   res.Name,
				Type:   res.Type,
				Prune:  res.Prune,
				Params: res.Params,
			})
		}

		if err := eng.Prune(allItems, createFn); err != nil {
			pterm.Error.Printf("Prune failed: %v\n", err)
			return err
		}
	}

	pterm.Println()
	pterm.DefaultBasicText.WithStyle(pterm.NewStyle(pterm.FgGreen, pterm.Bold)).Println("✨ Configuration applied successfully!")
	return nil
}
