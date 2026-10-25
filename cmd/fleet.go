package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/fleet"
	"github.com/melih-ucgun/veto/internal/inventory"
	"github.com/melih-ucgun/veto/internal/system"
	"github.com/melih-ucgun/veto/internal/transport"
	atomic "github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// fleetCmd represents the fleet command
var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Manage a fleet of servers",
	Long:  `Inventory based operations for multiple hosts.`,
}

// factsCmd represents the facts command
var factsCmd = &cobra.Command{
	Use:   "facts",
	Short: "Gather system facts from all hosts",
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Load Inventory
		invFile, _ := cmd.Flags().GetString("inventory")
		if invFile == "" {
			invFile = "inventory.yaml"
		}

		inv, err := loadInventory(invFile)
		if err != nil {
			atomic.Error.Printf("Failed to load inventory: %v\n", err)
			return
		}

		// 2. Gather Facts concurrently
		type HostFact struct {
			HostName string
			OS       string
			Kernel   string
			CPU      string
			RAM      string
			Status   string
			Error    error
		}

		results := make(chan HostFact, len(inv.Hosts))
		var wg sync.WaitGroup

		// Pre-flight: Ensure we have sudo passwords if needed
		// For facts gathering, we Force sudo usage usually, or we should respect inventory?
		// The code below hardcodes BecomeMethod="sudo". So we should ensure key exists.
		for i := range inv.Hosts {
			if inv.Hosts[i].Vars == nil {
				inv.Hosts[i].Vars = make(map[string]string)
			}
			// Force become method for facts, or just set it so prompt triggers
			inv.Hosts[i].Vars["ansible_become_method"] = "sudo"
		}

		if err := ensureSudoPasswords(inv.Hosts); err != nil {
			atomic.Error.Printf("Auth Error: %v\n", err)
			return
		}

		spinner, _ := atomic.DefaultSpinner.Start(fmt.Sprintf("Gathering facts from %d hosts...", len(inv.Hosts)))

		for _, host := range inv.Hosts {
			wg.Add(1)
			go func(h inventory.Host) {
				defer wg.Done()

				// Map inventory.Host to transport.HostConfig for Transport
				// TODO: Load vars for Become info if needed
				cfgHost := transport.HostConfig{
					Name:           h.Name,
					Address:        h.Address,
					User:           h.User,
					Port:           h.Port,
					SSHKeyPath:     h.KeyPath,
					BecomeMethod:   "sudo",
					BecomePassword: h.Vars["ansible_become_password"],
					SSHPassword:    h.Vars["ansible_ssh_pass"],

					KnownHostsPath:        h.KnownHosts,
					InsecureIgnoreHostKey: h.InsecureHostKey,
				}

				// Create Transport
				// TODO: Better context management with timeouts
				ctx := core.NewSystemContext(false, nil) // Base context

				// Initialize Transport
				var tr core.Transport
				var err error
				if h.Connection == "ssh" {
					tr, err = transport.NewSSHTransport(ctx.Context, cfgHost)
				} else {
					tr = transport.NewLocalTransport()
				}

				if err != nil {
					results <- HostFact{HostName: h.Name, Status: "OFFLINE", Error: err}
					return
				}
				defer tr.Close()

				// Update Context
				ctx.Transport = tr
				// CRITICAL: Set FS from Transport!
				ctx.FS = tr.GetFileSystem()

				// Run Detection
				// Panic safety?
				defer func() {
					if r := recover(); r != nil {
						results <- HostFact{HostName: h.Name, Status: "PANIC", Error: fmt.Errorf("%v", r)}
					}
				}()

				system.Detect(ctx)

				results <- HostFact{
					HostName: h.Name,
					OS:       fmt.Sprintf("%s %s", ctx.Distro, ctx.Version),
					Kernel:   ctx.Kernel,
					CPU:      ctx.Hardware.CPUModel,
					RAM:      ctx.Hardware.RAMTotal,
					Status:   "ONLINE",
				}
			}(host)
		}

		wg.Wait()
		close(results)
		spinner.Success("Facts gathered")

		// 3. Display Table
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "HOST\tSTATUS\tOS\tKERNEL\tCPU\tRAM")
		fmt.Fprintln(w, "----\t------\t--\t------\t---\t---")

		for res := range results {
			statusIcon := "✅"
			if res.Status != "ONLINE" {
				statusIcon = "❌"
			}

			// Truncate CPU for display
			cpuDisplay := res.CPU
			if len(cpuDisplay) > 30 {
				cpuDisplay = cpuDisplay[:27] + "..."
			}

			fmt.Fprintf(w, "%s\t%s %s\t%s\t%s\t%s\t%s\n",
				res.HostName,
				statusIcon, res.Status,
				res.OS,
				res.Kernel,
				cpuDisplay,
				res.RAM,
			)
		}
		w.Flush()
	},
}

// execCmd represents the exec command
var execCmd = &cobra.Command{
	Use:   "exec -- [command]",
	Short: "Run arbitrary command on all hosts",
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Load Inventory
		invFile, _ := cmd.Flags().GetString("inventory")
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		sudo, _ := cmd.Flags().GetBool("sudo")

		if invFile == "" {
			invFile = "inventory.yaml"
		}

		inv, err := loadInventory(invFile)
		if err != nil {
			atomic.Error.Printf("Failed to load inventory: %v\n", err)
			return
		}

		// 2. Parse Command
		// Args after double dash are treated as command
		if len(args) == 0 {
			atomic.Error.Println("No command specified. Usage: veto fleet exec -- \"uptime\"")
			return
		}
		command := strings.Join(args, " ")

		// 3. Auth Check
		if sudo {
			if err := ensureSudoPasswords(inv.Hosts); err != nil {
				atomic.Error.Printf("Auth Error: %v\n", err)
				return
			}
		}

		// 4. Validate Hosts keys/connection
		// (Skipped for speed, Executor handles per-host connection errors)

		// 5. Build & Run Executor
		// We need to import the new fleet package.
		// Since we are in 'cmd' package, and 'fleetCmd' var is here, we are good.
		// But wait, 'internal/fleet' package name conflicts with 'fleetCmd' var name semantically?
		// No, `fleet` package is imported as `fleet` (if we alias it or verify imports).
		// Currently `cmd/fleet.go` imports `github.com/melih-ucgun/veto/internal/system` etc.
		// I need to start using `github.com/melih-ucgun/veto/internal/fleet`.
		// But wait, `cmd/fleet.go` ALREADY imports `internal/fleet` (implied by file content I saw earlier? No wait, let's check).
		// I saw earlier: `github.com/melih-ucgun/veto/internal/fleet` // New import needed maybe?
		// Previous `apply` command imported `fleet`. So it should be fine.

		// However, I need to make sure I add the import if it's missing or use the existing alias.
		// Let's rely on `goimports` behavior or manual check.
		// Since I'm using `replace_file_content`, I should probably check imports first.
		// `cmd/fleet.go` previously imported `internal/core`, `inventory`, `system`, `transport`.
		// I need to ensure `github.com/melih-ucgun/veto/internal/fleet` is imported.

		exec := fleet.NewExecutor(inv.Hosts, concurrency, sudo)
		if err := exec.Run(command); err != nil {
			// Error is already printed by Executor summary, but command exit code should reflect it
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(fleetCmd)
	fleetCmd.AddCommand(factsCmd)
	fleetCmd.AddCommand(execCmd)
	fleetCmd.PersistentFlags().StringP("inventory", "i", "inventory.yaml", "Path to inventory file")
	execCmd.Flags().IntP("concurrency", "C", 10, "Number of concurrent hosts")
	execCmd.Flags().Bool("sudo", false, "Run with sudo privileges")
}

// loadInventory loads the inventory and, unless --decrypt=false, decrypts
// ENC[...] host vars (e.g. ansible_ssh_pass) with the master key.
func loadInventory(path string) (*inventory.Inventory, error) {
	inv, err := inventory.LoadInventory(path)
	if err != nil {
		return nil, err
	}
	if decrypt, _ := rootCmd.PersistentFlags().GetBool("decrypt"); decrypt && inv.HasEncryptedVars() {
		key := getMasterKey()
		if key == "" {
			return nil, fmt.Errorf("inventory contains encrypted values but no master key was found")
		}
		if err := inv.DecryptVars(key); err != nil {
			return nil, err
		}
	}
	return inv, nil
}
