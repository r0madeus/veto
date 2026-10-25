package core

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pterm/pterm"

	"github.com/r0madeus/veto/internal/state"
	"github.com/r0madeus/veto/internal/types"
)

// StateUpdater interface allows Engine to be independent of the state package.
type StateUpdater interface {
	UpdateResource(resType, name, targetState, status string) error
	AddTransaction(tx types.Transaction) error
}

// ConfigItem is the raw configuration part that the engine will process.
type ConfigItem struct {
	Name      string
	Type      string
	State     string
	When      string // Condition to evaluate
	Params    map[string]interface{}
	Hooks     Hooks
	Prune     bool     `yaml:"prune"`
	DependsOn []string `yaml:"depends_on"`
}

// Hooks defines lifecycle hooks for a resource execution.
type Hooks struct {
	Pre      string
	Post     string
	OnChange string
	OnFail   string
}

// Engine is the main structure managing resources.
type Engine struct {
	Context        *SystemContext
	StateUpdater   StateUpdater // Optional: State manager
	AppliedHistory []Resource

	// packageMu serializes package manager operations within a parallel layer:
	// apt/dpkg, pacman, dnf etc. hold a global lock and fail when run concurrently.
	packageMu sync.Mutex

	// OnLayerStart, if set, is called before each layer of RunLayers. The
	// returned func (may be nil) is called with that layer's result.
	OnLayerStart func(index, total int, layer []ConfigItem) (done func(err error))

	// tx is the transaction shared by all layers run on this engine.
	tx *types.Transaction
}

// packageManagerTypes are resource types that drive a system package manager.
var packageManagerTypes = map[string]bool{
	"package": true, "pkg": true, "bundle": true,
	"pacman": true, "yay": true, "paru": true, "apt": true, "dnf": true,
	"yum": true, "zypper": true, "apk": true, "brew": true, "snap": true, "flatpak": true,
}

func isPackageManagerType(resType string) bool {
	return packageManagerTypes[resType]
}

// NewEngine creates a new engine instance.
func NewEngine(ctx *SystemContext, updater StateUpdater) *Engine {
	// Initialize Backup Manager
	_ = InitBackupManager(ctx.FS) // Ignore error for now (or log)
	return &Engine{
		Context:      ctx,
		StateUpdater: updater,
	}
}

// ResourceCreator fonksiyon tipi
type ResourceCreator func(resType, name string, params map[string]interface{}, ctx *SystemContext) (Resource, error)

// Run processes the given configuration list. Items without dependencies keep
// their file order (one item per layer); otherwise the dependency graph decides
// the layers. All layers share a single transaction.
func (e *Engine) Run(items []ConfigItem, createFn ResourceCreator) error {
	layers, err := e.buildLayers(items)
	if err != nil {
		return err
	}
	e.tx = nil // a Run is one fresh transaction
	return e.RunLayers(layers, createFn)
}

// buildLayers splits items into execution layers.
func (e *Engine) buildLayers(items []ConfigItem) ([][]ConfigItem, error) {
	hasDeps := false
	for _, item := range items {
		if len(item.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}

	// No dependencies: preserve exact file order.
	if !hasDeps {
		layers := make([][]ConfigItem, len(items))
		for i, item := range items {
			layers[i] = []ConfigItem{item}
		}
		return layers, nil
	}

	graph := NewGraph()
	if err := graph.BuildGraph(items); err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}
	layers, err := graph.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("dependency error: %w", err)
	}
	e.Context.Logger.Info(fmt.Sprintf("Executing %d layers of resources", len(layers)))
	return layers, nil
}

// RunLayers executes the layers in order. The first failing layer is rolled
// back together with everything applied before it and stops the run. In
// dry-run nothing is applied, so the remaining layers are still checked.
func (e *Engine) RunLayers(layers [][]ConfigItem, createFn ResourceCreator) error {
	var lastErr error
	for i, layer := range layers {
		e.Context.Logger.Debug(fmt.Sprintf("Executing Layer %d (%d resources)", i+1, len(layer)))
		var done func(error)
		if e.OnLayerStart != nil {
			done = e.OnLayerStart(i, len(layers), layer)
		}
		err := e.RunParallel(layer, createFn)
		if done != nil {
			done(err)
		}
		if err != nil {
			if !e.Context.DryRun {
				return err
			}
			lastErr = err
		}
	}
	return lastErr
}

// currentTransaction returns the transaction shared by all layers run on this
// engine, creating it on first use.
func (e *Engine) currentTransaction() *types.Transaction {
	if e.tx == nil {
		e.tx = &types.Transaction{
			ID:        uuid.New().String(),
			Timestamp: time.Now(),
			Status:    "success",
			Changes:   []types.TransactionChange{},
		}
		e.Context.TxID = e.tx.ID
		e.Context.BackupManager = state.NewBackupManager("") // Use default path
	}
	return e.tx
}

// itemOutcome is the result of processing one config item.
type itemOutcome struct {
	res    Resource
	change *types.TransactionChange // set only when the resource changed the system
	err    error
}

// RunParallel processes the items of one layer in parallel. Layers run on the
// same engine share one transaction; a failure rolls back this layer and all
// previously applied layers.
func (e *Engine) RunParallel(layer []ConfigItem, createFn ResourceCreator) error {
	tx := e.currentTransaction()

	outcomes := make([]itemOutcome, len(layer))
	var wg sync.WaitGroup
	for i := range layer {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outcomes[i] = e.processItem(layer[i], createFn)
		}(i)
	}
	wg.Wait()

	var applied []Resource // successful changes of this layer, in config order
	errCount := 0
	for _, o := range outcomes {
		if o.change != nil {
			tx.Changes = append(tx.Changes, *o.change)
		}
		switch {
		case o.err != nil:
			errCount++
		case o.change != nil && !e.Context.DryRun:
			applied = append(applied, o.res)
		}
	}

	if errCount > 0 {
		tx.Status = "failed"
		if !e.Context.DryRun {
			pterm.Println()
			pterm.Error.Println("Error occurred. Initiating Rollback...")

			// 1. Revert what succeeded in the current layer
			pterm.Warning.Printf("Rolling back current layer (%d resources)...\n", len(applied))
			e.rollback(applied)

			// 2. Revert everything applied in previous layers
			pterm.Warning.Printf("Rolling back previous layers (%d resources)...\n", len(e.AppliedHistory))
			e.rollback(e.AppliedHistory)
			e.AppliedHistory = nil

			tx.Status = "reverted"
		}
	} else {
		// Rollback is LIFO over this list, so it grows in execution order.
		e.AppliedHistory = append(e.AppliedHistory, applied...)
	}

	// Persist (upsert by ID: later layers update the same history entry)
	if !e.Context.DryRun && e.StateUpdater != nil {
		snapshot := *tx
		snapshot.Changes = append([]types.TransactionChange(nil), tx.Changes...)
		if err := e.StateUpdater.AddTransaction(snapshot); err != nil {
			e.Context.Logger.Warn(fmt.Sprintf("Failed to save history: %v", err))
		}
	}

	if errCount > 0 {
		return fmt.Errorf("encountered %d errors during execution", errCount)
	}
	return nil
}

// processItem runs the full lifecycle of one config item: condition, template
// rendering, creation, validation, hooks, diff, apply and state update.
func (e *Engine) processItem(it ConfigItem, createFn ResourceCreator) itemOutcome {
	log := e.Context.Logger

	// Params preparation (copy so callers' items are not mutated)
	params := make(map[string]interface{}, len(it.Params)+2)
	for k, v := range it.Params {
		params[k] = v
	}
	params["state"] = it.State
	params["prune"] = it.Prune
	it.Params = params

	// 0. Check Condition (When)
	if it.When != "" {
		shouldRun, err := EvaluateCondition(it.When, e.Context)
		if err != nil {
			log.Error(fmt.Sprintf("[%s] Condition Error: %v", it.Name, err))
			return itemOutcome{err: err}
		}
		if !shouldRun {
			log.Debug(fmt.Sprintf("[%s] Skipped (Condition not met: %s)", it.Name, it.When))
			return itemOutcome{}
		}
	}

	// 0.5 Render Templates in Params
	if err := renderParams(it.Params, e.Context); err != nil {
		log.Error(fmt.Sprintf("[%s] Template Error: %v", it.Name, err))
		return itemOutcome{err: err}
	}

	// 1. Create resource
	res, err := createFn(it.Type, it.Name, it.Params, e.Context)
	if err != nil {
		log.Error(fmt.Sprintf("[%s] Invalid resource definition: %v", it.Name, err))
		return itemOutcome{err: err}
	}

	// 1.5 Validate resource configuration
	if err := res.Validate(e.Context); err != nil {
		log.Error(fmt.Sprintf("[%s] Validation Failed: %v", it.Name, err))
		return itemOutcome{err: err}
	}

	// 1.9 PRE-HOOK
	if it.Hooks.Pre != "" {
		if err := executeHook(e.Context, it.Hooks.Pre); err != nil {
			log.Error(fmt.Sprintf("[%s] Pre-Hook Failed: %v. Skipping resource.", it.Name, err))
			return itemOutcome{err: err}
		}
		log.Debug(fmt.Sprintf("[%s] Pre-Hook executed", it.Name))
	}

	// Package managers hold a global lock; never run two at once.
	if isPackageManagerType(it.Type) {
		e.packageMu.Lock()
	}

	// 2. Capture Diff (if supported) BEFORE Apply
	var pendingDiff string
	if differ, ok := res.(Differ); ok {
		if d, err := differ.Diff(e.Context); err == nil {
			pendingDiff = d
		}
	}

	// 2. Apply resource
	result, applyErr := res.Apply(e.Context)

	if isPackageManagerType(it.Type) {
		e.packageMu.Unlock()
	}

	// 2.1 POST-HOOK (runs whenever Apply was attempted; failure only warns)
	if it.Hooks.Post != "" {
		if hookErr := executeHook(e.Context, it.Hooks.Post); hookErr != nil {
			log.Warn(fmt.Sprintf("[%s] Post-Hook Failed: %v", it.Name, hookErr))
		} else {
			log.Debug(fmt.Sprintf("[%s] Post-Hook executed", it.Name))
		}
	}

	out := itemOutcome{res: res}
	status := "success"

	switch {
	case applyErr != nil:
		status = "failed"
		out.err = applyErr
		log.Error(fmt.Sprintf("[%s] %s: Failed: %v", it.Type, it.Name, applyErr))

		// 2.2 ON-FAIL HOOK
		if it.Hooks.OnFail != "" {
			if hookErr := executeHook(e.Context, it.Hooks.OnFail); hookErr != nil {
				log.Warn(fmt.Sprintf("[%s] On-Fail Hook Failed: %v", it.Name, hookErr))
			}
		}

	case result.Changed:
		log.Info(fmt.Sprintf("[%s] %s: %s", it.Type, it.Name, result.Message))

		// 2.3 ON-CHANGE HOOK
		if it.Hooks.OnChange != "" {
			if hookErr := executeHook(e.Context, it.Hooks.OnChange); hookErr != nil {
				log.Warn(fmt.Sprintf("[%s] On-Change Hook Failed: %v", it.Name, hookErr))
			} else {
				log.Info(fmt.Sprintf("   └── Hook: %s", it.Hooks.OnChange))
			}
		}

		change := types.TransactionChange{
			Type:   it.Type,
			Name:   it.Name,
			Action: "applied",
			Diff:   pendingDiff,
			Target: it.Name, // Fallback
		}
		if p, ok := it.Params["path"].(string); ok {
			change.Target = p
		}

		// Local interface to avoid an import cycle
		type Backupable interface {
			GetBackupPath() string
		}
		if b, ok := res.(Backupable); ok {
			change.BackupPath = b.GetBackupPath()
		}
		out.change = &change

	default:
		msg := "OK"
		if result.Message != "" {
			msg = result.Message
		}
		log.Debug(fmt.Sprintf("[%s] %s: %s", it.Type, it.Name, msg))
	}

	// 3. Save State. A failed attempt is recorded as "failed" too.
	if !e.Context.DryRun && e.StateUpdater != nil {
		if err := e.StateUpdater.UpdateResource(it.Type, it.Name, it.State, status); err != nil {
			log.Warn(fmt.Sprintf("[%s] Failed to save state: %v", it.Name, err))
		}
	}

	return out
}

// PlanResult represents the outcome of a Plan operation.
type PlanResult struct {
	Changes []PlanChange
}

// PlanChange represents a single proposed change.
type PlanChange struct {
	Type   string
	Name   string
	Action string // "create", "modify", "noop"
	Diff   string // Detailed diff for files/templates
}

// Plan generates a preview of changes without applying them.
func (e *Engine) Plan(items []ConfigItem, createFn ResourceCreator) (*PlanResult, error) {
	result := &PlanResult{
		Changes: []PlanChange{},
	}

	for _, item := range items {
		// Params preparation
		if item.Params == nil {
			item.Params = make(map[string]interface{})
		}
		item.Params["state"] = item.State

		// 0. Check Condition (When)
		if item.When != "" {
			shouldRun, err := EvaluateCondition(item.When, e.Context)
			if err != nil {
				return nil, fmt.Errorf("[%s] Condition Error: %w", item.Name, err)
			}
			if !shouldRun {
				continue // Skip silently or add as "skipped"
			}
		}

		// 0.5 Render Templates
		if err := renderParams(item.Params, e.Context); err != nil {
			return nil, fmt.Errorf("[%s] Template Error: %w", item.Name, err)
		}

		// 1. Create resource
		resApp, err := createFn(item.Type, item.Name, item.Params, e.Context)
		if err != nil {
			return nil, fmt.Errorf("[%s] Creation Error: %w", item.Name, err)
		}

		// 1.5 Validate resource configuration
		if err := resApp.Validate(e.Context); err != nil {
			return nil, fmt.Errorf("[%s] Validation Error: %w", item.Name, err)
		}

		// 2. Check State
		var action string
		var diff string

		if checker, ok := resApp.(interface {
			Check(ctx *SystemContext) (bool, error)
		}); ok {
			needsAction, err := checker.Check(e.Context)
			if err != nil {
				return nil, fmt.Errorf("[%s] Check Error: %w", item.Name, err)
			}

			if needsAction {
				action = "apply"
				// If it supports Diff, get detailed changes
				if differ, ok := resApp.(Differ); ok {
					if d, err := differ.Diff(e.Context); err == nil {
						diff = d
					}
				}
			} else {
				action = "noop"
			}
		} else {
			action = "unknown"
		}

		if action != "noop" {
			result.Changes = append(result.Changes, PlanChange{
				Type:   item.Type,
				Name:   item.Name,
				Action: action,
				Diff:   diff,
			})
		}
	}

	return result, nil
}

// rollback reverts the given list of resources in reverse order.
func (e *Engine) rollback(resources []Resource) {
	// Go in reverse order
	for i := len(resources) - 1; i >= 0; i-- {
		res := resources[i]
		if rev, ok := res.(Revertable); ok {
			pterm.Warning.Printf("Visualizing Rollback for %s...\n", res.GetName())
			if err := rev.Revert(e.Context); err != nil {
				pterm.Error.Printf("Failed to revert %s: %v\n", res.GetName(), err)
				if !e.Context.DryRun && e.StateUpdater != nil {
					_ = e.StateUpdater.UpdateResource(res.GetType(), res.GetName(), "any", "revert_failed")
				}
			} else {
				pterm.Success.Printf("Reverted %s\n", res.GetName())
				if !e.Context.DryRun && e.StateUpdater != nil {
					// Successful revert, mark as 'reverted'
					_ = e.StateUpdater.UpdateResource(res.GetType(), res.GetName(), "any", "reverted")
				}
			}
		}
	}
}

// renderParams traverses the map and renders any string values as templates.
func renderParams(params map[string]interface{}, ctx *SystemContext) error {
	for k, v := range params {
		switch val := v.(type) {
		case string:
			rendered, err := ExecuteTemplate(val, ctx)
			if err != nil {
				return fmt.Errorf("param '%s': %w", k, err)
			}
			params[k] = rendered
		case map[string]interface{}:
			// Recursive
			if err := renderParams(val, ctx); err != nil {
				return err
			}
		case []interface{}:
			// Iterate slice
			for i, item := range val {
				if str, ok := item.(string); ok {
					rendered, err := ExecuteTemplate(str, ctx)
					if err != nil {
						return fmt.Errorf("param '%s' index %d: %w", k, i, err)
					}
					val[i] = rendered
				} else if subMap, ok := item.(map[string]interface{}); ok {
					if err := renderParams(subMap, ctx); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Prune identifies unmanaged resources and removes them upon confirmation.
// Supports any resource type that implements the Lister interface.
func (e *Engine) Prune(configItems []ConfigItem, createFn ResourceCreator) error {
	pterm.Println()
	pterm.DefaultHeader.WithFullWidth().
		WithBackgroundStyle(pterm.NewStyle(pterm.BgRed)).
		WithTextStyle(pterm.NewStyle(pterm.FgWhite, pterm.Bold)).
		Println("PRUNE MODE (Destructive)")

	totalUnmanaged := 0
	type PruneTask struct {
		Type      string
		Resources []string
		Adapter   Lister
	}
	var tasks []PruneTask

	// 1. GLOBAL PRUNING (Packages, Services)
	// These types are managed globally: anything installed but not in config is unmanaged.
	globalTypes := []string{"package", "service"}

	for _, resType := range globalTypes {
		managed := make(map[string]bool)
		for _, item := range configItems {
			// Check for both the generic 'package' type and provider-specific names (pacman, apt, etc.)
			if item.Type == resType || (resType == "package" && (item.Type == "pkg" || item.Type == "pacman" || item.Type == "apt" || item.Type == "dnf" || item.Type == "brew" || item.Type == "apk" || item.Type == "zypper" || item.Type == "yum")) {
				managed[item.Name] = true
			}
		}

		// Create dummy instance to get the appropriate Lister for the current OS
		dummyRes, err := createFn(resType, "prune_helper", nil, e.Context)
		if err != nil {
			continue
		}

		lister, ok := dummyRes.(Lister)
		if !ok {
			continue
		}

		e.Context.Logger.Info(fmt.Sprintf("Analyzing global %s resources...", resType))
		installed, err := lister.ListInstalled(e.Context)
		if err != nil {
			e.Context.Logger.Warn(fmt.Sprintf("Failed to list installed %s: %v", resType, err))
			continue
		}

		var unmanaged []string
		for _, name := range installed {
			if !managed[name] {
				// Safety: Skip core system packages if needed (optional implementation plan detail)
				unmanaged = append(unmanaged, name)
			}
		}

		if len(unmanaged) > 0 {
			tasks = append(tasks, PruneTask{Type: resType, Resources: unmanaged, Adapter: lister})
			totalUnmanaged += len(unmanaged)
			e.Context.Logger.Warn("Found unmanaged resources", "count", len(unmanaged), "type", resType)
		}
	}

	// 2. SCOPED PRUNING (Files in specific directories)
	// These are only pruned if explicitly marked with prune: true in config.
	for _, item := range configItems {
		if !item.Prune {
			continue
		}

		// Currently only file supports scoped pruning
		if item.Type != "file" {
			continue
		}

		res, err := createFn(item.Type, item.Name, item.Params, e.Context)
		if err != nil {
			continue
		}

		lister, ok := res.(Lister)
		if !ok {
			continue
		}

		e.Context.Logger.Info(fmt.Sprintf("Analyzing scoped %s: %s", item.Type, item.Name))
		installed, err := lister.ListInstalled(e.Context)
		if err != nil {
			e.Context.Logger.Warn(fmt.Sprintf("Failed to list scoped %s: %v", item.Name, err))
			continue
		}

		// For scoped pruning, "managed" are all file resources THAT FALL UNDER THIS SCOPE.
		// Since we handle file resources individually, we need to check if they point to paths
		// inside this directory.
		managedPaths := make(map[string]bool)
		for _, otherItem := range configItems {
			if otherItem.Type == "file" {
				path, _ := otherItem.Params["path"].(string)
				if path == "" {
					path = otherItem.Name
				}
				managedPaths[filepath.Clean(path)] = true
			}
		}

		var unmanaged []string
		for _, path := range installed {
			if !managedPaths[filepath.Clean(path)] {
				unmanaged = append(unmanaged, path)
			}
		}

		if len(unmanaged) > 0 {
			tasks = append(tasks, PruneTask{Type: item.Type, Resources: unmanaged, Adapter: lister})
			totalUnmanaged += len(unmanaged)
			e.Context.Logger.Warn("Found unmanaged files in scoped directory", "count", len(unmanaged), "path", item.Name)
		}
	}

	if totalUnmanaged == 0 {
		pterm.Success.Println("System is clean! No unmanaged resources found.")
		return nil
	}

	// Preview & Confirmation
	pterm.Println()
	pterm.Warning.Printf("Total unmanaged resources found: %d\n", totalUnmanaged)
	for _, task := range tasks {
		pterm.Info.Printf("Type [%s]: %d items\n", task.Type, len(task.Resources))
		for i, name := range task.Resources {
			if i < 3 {
				fmt.Printf(" - %s\n", name)
			} else if i == 3 {
				fmt.Printf(" ... and %d more\n", len(task.Resources)-3)
				break
			}
		}
	}

	pterm.Println()
	confirm, _ := pterm.DefaultInteractiveConfirm.
		WithDefaultText("Do you want to proceed with pruning?").
		WithDefaultValue(false).
		Show()

	if !confirm {
		pterm.Info.Println("Prune cancelled.")
		return nil
	}

	// Execution
	for _, task := range tasks {
		if batchRemover, ok := task.Adapter.(BatchRemover); ok && task.Type != "file" {
			e.Context.Logger.Info("Batch pruning %d %s resources...", len(task.Resources), task.Type)
			if err := batchRemover.RemoveBatch(task.Resources, e.Context); err != nil {
				e.Context.Logger.Warn("Batch removal failed: %v. Falling back to individual removal.", err)
			} else {
				continue
			}
		}

		for _, name := range task.Resources {
			e.Context.Logger.Info("Pruning [%s] %s", task.Type, name)
			if e.Context.DryRun {
				continue
			}

			params := make(map[string]interface{})
			if task.Type == "service" {
				params["enabled"] = false
				params["state"] = "stopped"
			} else if task.Type == "file" {
				params["state"] = "absent"
				params["path"] = name
			} else {
				params["state"] = "absent"
			}

			res, err := createFn(task.Type, name, params, e.Context)
			if err != nil {
				e.Context.Logger.Error("Failed to create prune handle for %s: %v", name, err)
				continue
			}

			_, err = res.Apply(e.Context)
			if err != nil {
				e.Context.Logger.Error("Failed to prune %s: %v", name, err)
			}
		}
	}

	pterm.Success.Println("Prune completed successfully.")
	return nil
}

// executeHook executes a shell command using the context's transport.
func executeHook(ctx *SystemContext, cmd string) error {
	if ctx.DryRun {
		ctx.Logger.Info(fmt.Sprintf("[DryRun] Would execute hook: %s", cmd))
		return nil
	}
	// Use Transport to execute
	out, err := ctx.Transport.Execute(ctx.Context, cmd)
	if err != nil {
		return fmt.Errorf("command '%s' failed: %w, output: %s", cmd, err, string(out))
	}
	return nil
}
