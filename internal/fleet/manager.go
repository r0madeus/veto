package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/inventory"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
)

// FleetManager orchestrates operations across multiple hosts.
type FleetManager struct {
	Hosts    []inventory.Host
	DiffMode bool // For Plan mode
	DryRun   bool
	Prune    bool
	Logger   core.Logger
}

// NewFleetManager creates a new FleetManager.
func NewFleetManager(hosts []inventory.Host, dryRun bool, prune bool, logger core.Logger) *FleetManager {
	return &FleetManager{
		Hosts:  hosts,
		DryRun: dryRun,
		Prune:  prune,
		Logger: logger,
	}
}

// ApplyConfig executes the given plan on all hosts in parallel.
// It accepts layers of ConfigItems (sorted by dependency).
func (f *FleetManager) ApplyConfig(layers [][]core.ConfigItem, concurrency int, createFn core.ResourceCreator) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(f.Hosts))
	sem := make(chan struct{}, concurrency) // Semaphore for concurrency control

	pterm.DefaultSection.Printf("Fleet Deployment: %d hosts (Concurrency: %d)", len(f.Hosts), concurrency)

	for _, host := range f.Hosts {
		wg.Add(1)
		go func(h inventory.Host) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			// Initialize Scoped Logger for this host
			hostLogger := f.Logger.With("host", h.Name)
			hostLogger.Info("Connecting to host")

			// 1. Initialize Transport
			var trans core.Transport
			var err error

			if h.Connection == "local" {
				trans = transport.NewLocalTransport()
			} else {
				// Determine port
				port := h.Port
				if port == 0 {
					port = 22
				}

				sshConfig := transport.HostConfig{
					Name:           h.Name,
					Address:        h.Address,
					User:           h.User,
					Port:           port,
					SSHKeyPath:     h.KeyPath,
					BecomeMethod:   h.Vars["ansible_become_method"],
					BecomePassword: h.Vars["ansible_become_password"],
					SSHPassword:    h.Vars["ansible_ssh_pass"],

					KnownHostsPath:        h.KnownHosts,
					InsecureIgnoreHostKey: h.InsecureHostKey,
				}
				// Set timeout for connection
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()

				trans, err = transport.NewSSHTransport(ctx, sshConfig)
				if err != nil {
					hostLogger.Error("Connection Failed: %v", err)
					errChan <- fmt.Errorf("[%s] connection failed: %w", h.Name, err)
					return
				}
			}
			defer trans.Close()

			// 2. Setup Context
			sysCtx := &core.SystemContext{
				Context:    context.Background(),
				Transport:  trans,
				FS:         trans.GetFileSystem(),
				DryRun:     f.DryRun,
				TargetUser: h.User,
				Logger:     hostLogger,
			}

			// 3. Detect System
			system.Detect(sysCtx)
			hostLogger.Info("System detected: %s %s", sysCtx.Distro, sysCtx.Version)

			// 4. Create Engine
			engine := core.NewEngine(sysCtx, nil) // State updater per host TODO

			// 5. Execute Layers
			// Merge host vars into context Vars
			if sysCtx.Vars == nil {
				sysCtx.Vars = make(map[string]string)
			}
			for k, v := range h.Vars {
				sysCtx.Vars[k] = v
			}

			// Deep copy layer params for this host
			hostLayers := make([][]core.ConfigItem, len(layers))
			for i, layer := range layers {
				hostLayers[i] = make([]core.ConfigItem, len(layer))
				for j, item := range layer {
					newItem := item
					newItem.Params = make(map[string]interface{}, len(item.Params))
					for k, v := range item.Params {
						newItem.Params[k] = v
					}
					hostLayers[i][j] = newItem
				}
			}

			// One transaction per host; a failure rolls back every layer.
			failedLayer := 0
			engine.OnLayerStart = func(i, total int, layer []core.ConfigItem) func(error) {
				return func(err error) {
					if err != nil {
						failedLayer = i + 1
					}
				}
			}
			if err := engine.RunLayers(hostLayers, createFn); err != nil {
				hostLogger.Error("Layer %d Failed: %v", failedLayer, err)
				errChan <- fmt.Errorf("[%s] layer %d failed: %w", h.Name, failedLayer, err)
				return // Stop this host
			}

			hostLogger.Info("Completed Successfully")

		}(host)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	errCount := 0
	for range errChan {
		errCount++
	}

	if errCount > 0 {
		return fmt.Errorf("fleet execution failed on %d hosts", errCount)
	}
	pterm.Success.Println("Fleet execution completed successfully.")
	return nil
}
