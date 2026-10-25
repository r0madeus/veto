package core

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pterm/pterm"

	"github.com/melih-ucgun/veto/internal/state"
	"github.com/melih-ucgun/veto/internal/types"
)

// StateUpdater interface allows Engine to be independent of the state package.
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

// Run processes the given configuration list.
func (e *Engine) Run(items []ConfigItem, createFn ResourceCreator) error {
	// Check if any item has dependencies
	hasDeps := false
	for _, item := range items {
		if len(item.DependsOn) > 0 {
			hasDeps = true
			break
		}
	}

	// Legacy / Sequential Mode if no dependencies
	// This preserves exact behavior for existing configs that rely on file order
	if !hasDeps {
		return e.runSequential(items, createFn)
	}

	// DAG Mode
	graph := NewGraph()
	if err := graph.BuildGraph(items); err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	layers, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("dependency error: %w", err)
	}

	e.Context.Logger.Info(fmt.Sprintf("Executing %d layers of resources", len(layers)))

	for i, layer := range layers {
		e.Context.Logger.Debug(fmt.Sprintf("Executing Layer %d (%d resources)", i+1, len(layer)))
		if err := e.RunParallel(layer, createFn); err != nil {
			return err
		}
	}

	return nil
}

// runSequential is the legacy execution mode (moved from original Run)
func (e *Engine) runSequential(items []ConfigItem, createFn ResourceCreator) error {
	errCount := 0

	// Transaction recording
	transaction := types.Transaction{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Status:    "success",
		Changes:   []types.TransactionChange{},
	}

	// Initialize Backup Manager for this transaction
	e.Context.TxID = transaction.ID
	e.Context.BackupManager = state.NewBackupManager("") // Use default path

	for _, item := range items {
		// Params preparation
		if item.Params == nil {
			item.Params = make(map[string]interface{})
		}
		item.Params["state"] = item.State
		item.Params["prune"] = item.Prune

		// 0. Check Condition (When)
		if item.When != "" {
			shouldRun, err := EvaluateCondition(item.When, e.Context)
			if err != nil {
				e.Context.Logger.Error(fmt.Sprintf("[%s] Condition Error: %v", item.Name, err))
				errCount++
				continue
			}
			if !shouldRun {
				e.Context.Logger.Debug(fmt.Sprintf("[%s] Skipped (Condition not met)", item.Name))
				continue
			}
		}

		// 0.5 Render Templates in Params
		if err := renderParams(item.Params, e.Context); err != nil {
			e.Context.Logger.Error(fmt.Sprintf("[%s] Template Error: %v", item.Name, err))
			errCount++
			continue
		}

		// 1. Create resource
		res, err := createFn(item.Type, item.Name, item.Params, e.Context)
		if err != nil {
			Failure(err, "Skipping invalid resource definition: "+item.Name)
			errCount++
			continue
		}

		// 1.5 Validate resource configuration
		if err := res.Validate(e.Context); err != nil {
			e.Context.Logger.Error(fmt.Sprintf("[%s] Validation Failed: %v", item.Name, err))
			errCount++
			continue
		}

		// 1.9 PRE-HOOK
		if item.Hooks.Pre != "" {
			if err := executeHook(e.Context, item.Hooks.Pre); err != nil {
				e.Context.Logger.Error(fmt.Sprintf("[%s] Pre-Hook Failed: %v", item.Name, err))
				errCount++
				continue
			}
		}

		// 2. Capture Diff (if supported) BEFORE Apply
		var pendingDiff string
		if differ, ok := res.(Differ); ok {
			// We ignore error here as Diff might fail if resource is invalid, but we proceed to Apply which handles it
			if d, err := differ.Diff(e.Context); err == nil {
				pendingDiff = d
			}
		}

		// 2. Apply resource
		result, err := res.Apply(e.Context)

		// 2.1 POST-HOOK
		if item.Hooks.Post != "" {
			_ = executeHook(e.Context, item.Hooks.Post)
		}

		status := "success"
		if err != nil {
			status = "failed"
			errCount++
			e.Context.Logger.Error(fmt.Sprintf("[%s] Failed: %v", item.Name, err))

			// 2.2 ON-FAIL HOOK
			if item.Hooks.OnFail != "" {
				_ = executeHook(e.Context, item.Hooks.OnFail)
			}
		} else if result.Changed {
			e.Context.Logger.Info(fmt.Sprintf("[%s] %s", item.Name, result.Message))

			// 2.3 ON-CHANGE HOOK
			if item.Hooks.OnChange != "" {
				_ = executeHook(e.Context, item.Hooks.OnChange)
			}

			// Record change for History
			change := types.TransactionChange{
				Type:   item.Type,
				Name:   item.Name,
				Action: "applied",
				Diff:   pendingDiff,
			}

			// Try to get target path (specifically for file)
			if p, ok := item.Params["path"].(string); ok {
				change.Target = p
			} else {
				change.Target = item.Name // Fallback
			}

			// Use local interface to avoid import cycle
			type Backupable interface {
				GetBackupPath() string
			}

			if b, ok := res.(Backupable); ok {
				change.BackupPath = b.GetBackupPath()
			}

			transaction.Changes = append(transaction.Changes, change)
			// Add to history for rollback
			e.AppliedHistory = append(e.AppliedHistory, res)

		} else {
			msg := "OK"
			if result.Message != "" {
				msg = result.Message
			}
			e.Context.Logger.Debug(fmt.Sprintf("[%s] %s: %s", item.Type, item.Name, msg))
		}

		// 3. Save State (If not DryRun)
		if !e.Context.DryRun && e.StateUpdater != nil {
			// Save as "failed" even if it failed, to track the attempt
			saveErr := e.StateUpdater.UpdateResource(item.Type, item.Name, item.State, status)
			if saveErr != nil {
				fmt.Printf("⚠️ Warning: Failed to save state for %s: %v\n", item.Name, saveErr)
			}
		}
	}

	if errCount > 0 {
		transaction.Status = "failed"
	}

	// Save History
	if !e.Context.DryRun && e.StateUpdater != nil {
		if err := e.StateUpdater.AddTransaction(transaction); err != nil {
			fmt.Printf("⚠️ Warning: Failed to save history: %v\n", err)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("encountered %d errors during execution", errCount)
	}
	return nil
}

// RunParallel processes configuration items in the given layer in parallel.
func (e *Engine) RunParallel(layer []ConfigItem, createFn ResourceCreator) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(layer))
	var updatedResources []Resource // Track successful ones (For Rollback)
	var mu sync.Mutex               // lock for updatedResources

	// Transaction recording
	transaction := types.Transaction{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Status:    "success",
		Changes:   []types.TransactionChange{},
	}
	var txMu sync.Mutex

	// Initialize Backup Manager
	e.Context.TxID = transaction.ID
	e.Context.BackupManager = state.NewBackupManager("") // Use default path

	for _, item := range layer {
		wg.Add(1)
		go func(it ConfigItem) {
			defer wg.Done()

			// Params preparation
			if it.Params == nil {
				it.Params = make(map[string]interface{})
			}
			it.Params["state"] = it.State
			it.Params["prune"] = it.Prune

			// 0. Check Condition (When)
			if it.When != "" {
				shouldRun, err := EvaluateCondition(it.When, e.Context)
				if err != nil {
					pterm.Error.Printf("[%s] Condition Error: %v\n", it.Name, err)
					errChan <- err
					return
				}
				if !shouldRun {
					e.Context.Logger.Debug(fmt.Sprintf("[%s] Skipped (Condition not met: %s)", it.Name, it.When))
					return
				}
			}

			// 0.5 Render Templates in Params
			if err := renderParams(it.Params, e.Context); err != nil {
				e.Context.Logger.Error(fmt.Sprintf("[%s] Template Error: %v", it.Name, err))
				errChan <- err
				return
			}

			// 1. Create resource
			res, err := createFn(it.Type, it.Name, it.Params, e.Context)
			if err != nil {
				Failure(err, "Skipping invalid resource definition: "+it.Name)
				errChan <- err
				return
			}

			// 1.5 Validate resource configuration
			if err := res.Validate(e.Context); err != nil {
				e.Context.Logger.Error(fmt.Sprintf("[%s] Validation Failed: %v", it.Name, err))
				errChan <- err
				return
			}

			// 1.9 PRE-HOOK
			if it.Hooks.Pre != "" {
				if err := executeHook(e.Context, it.Hooks.Pre); err != nil {
					e.Context.Logger.Error(fmt.Sprintf("[%s] Pre-Hook Failed: %v. Skipping resource.", it.Name, err))
					errChan <- err
					return
				}
				e.Context.Logger.Debug(fmt.Sprintf("[%s] Pre-Hook executed", it.Name))
			}

			// Package managers must not run concurrently (lock contention).
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
			result, err := res.Apply(e.Context)

			if isPackageManagerType(it.Type) {
				e.packageMu.Unlock()
			}

			// 2.1 POST-HOOK (Always runs if Apply attempted, unless Pre failed)
			if it.Hooks.Post != "" {
				// We log warnings but don't fail the whole resource if Post hook fails?
				// Plan said: "If ... post ... hooks fail, we log a WARNING, but the main resource status remains"
				if hookErr := executeHook(e.Context, it.Hooks.Post); hookErr != nil {
					e.Context.Logger.Warn(fmt.Sprintf("[%s] Post-Hook Failed: %v", it.Name, hookErr))
				} else {
					e.Context.Logger.Debug(fmt.Sprintf("[%s] Post-Hook executed", it.Name))
				}
			}

			status := "success"

			if err != nil {
				status = "failed"
				errChan <- err
				e.Context.Logger.Error(fmt.Sprintf("[%s] %s: Failed: %v", it.Type, it.Name, err))

				// 2.2 ON-FAIL HOOK
				if it.Hooks.OnFail != "" {
					if hookErr := executeHook(e.Context, it.Hooks.OnFail); hookErr != nil {
						e.Context.Logger.Warn(fmt.Sprintf("[%s] On-Fail Hook Failed: %v", it.Name, hookErr))
					}
				}
			} else if result.Changed {
				// Success
				e.Context.Logger.Info(fmt.Sprintf("[%s] %s: %s", it.Type, it.Name, result.Message))

				// 2.3 ON-CHANGE HOOK
				if it.Hooks.OnChange != "" {
					if hookErr := executeHook(e.Context, it.Hooks.OnChange); hookErr != nil {
						e.Context.Logger.Warn(fmt.Sprintf("[%s] On-Change Hook Failed: %v", it.Name, hookErr))
					} else {
						e.Context.Logger.Info(fmt.Sprintf("   └── Hook: %s", it.Hooks.OnChange))
					}
				}

				// Save successful changes (For Rollback)
				if !e.Context.DryRun {
					mu.Lock()
					updatedResources = append(updatedResources, res)
					mu.Unlock()
				}

				// Record change for History
				change := types.TransactionChange{
					Type:   it.Type,
					Name:   it.Name,
					Action: "applied",
					Diff:   pendingDiff,
				}

				// Try to get target path
				if p, ok := it.Params["path"].(string); ok {
					change.Target = p
				} else {
					change.Target = it.Name // Fallback
				}

				// Use local interface to avoid import cycle
				type Backupable interface {
					GetBackupPath() string
				}

				if b, ok := res.(Backupable); ok {
					change.BackupPath = b.GetBackupPath()
				}

				txMu.Lock()
				transaction.Changes = append(transaction.Changes, change)
				txMu.Unlock()

			} else {
				// No Change (Info or Skipped)
				msg := "OK"
				if result.Message != "" {
					msg = result.Message
				}
				e.Context.Logger.Debug(fmt.Sprintf("[%s] %s: %s", it.Type, it.Name, msg))
			}

			// 3. Save State
			if !e.Context.DryRun && e.StateUpdater != nil {
				e.StateUpdater.UpdateResource(it.Type, it.Name, it.State, status)
			}
		}(item)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	errCount := 0
	for range errChan {
		errCount++
	}

	if errCount > 0 {
		transaction.Status = "failed"
		// Trigger Rollback
		if !e.Context.DryRun {
			pterm.Println()
			pterm.Error.Println("Error occurred. Initiating Rollback...")

			// 1. First revert operations that succeeded in the current layer (but incomplete due to other errors)
			pterm.Warning.Printf("Visualizing Rollback for current layer (%d resources)...\n", len(updatedResources))
			e.rollback(updatedResources)

			// 2. Revert operations completed in previous layers
			pterm.Warning.Printf("Visualizing Rollback for previous layers (%d resources)...\n", len(e.AppliedHistory))
			e.rollback(e.AppliedHistory)

			transaction.Status = "reverted"
		}
	}

	// Save History
	if !e.Context.DryRun && e.StateUpdater != nil {
		if err := e.StateUpdater.AddTransaction(transaction); err != nil {
			fmt.Printf("⚠️ Warning: Failed to save history: %v\n", err)
		}
	}

	if errCount > 0 {
		return fmt.Errorf("encountered %d errors in parallel layer execution", errCount)
	}

	// Add successful ones to global history
	// Note: Must be LIFO for Revert order. rollback function iterates specifically in reverse.
	// We add FIFO to AppliedHistory (append).
	e.AppliedHistory = append(e.AppliedHistory, updatedResources...)

	return nil
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
