package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/state"
	"github.com/r0madeus/veto/internal/types"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "View transaction history",
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := loadStateManager()
		if err != nil {
			pterm.Error.Printf("Failed to load state: %v\n", err)
			return
		}

		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).Println("Transaction Log")

		data := [][]string{
			{"ID", "Time", "Status", "Changes"},
		}

		// Reverse iteration for latest first
		if manager.Current != nil {
			for i := len(manager.Current.History) - 1; i >= 0; i-- {
				tx := manager.Current.History[i]
				status := tx.Status
				if status == "success" {
					status = pterm.FgGreen.Sprint(status)
				} else if status == "failed" {
					status = pterm.FgRed.Sprint(status)
				}

				data = append(data, []string{
					tx.ID[:8],
					tx.Timestamp.Format(time.RFC822),
					status,
					fmt.Sprintf("%d", len(tx.Changes)),
				})
			}
		}

		pterm.DefaultTable.WithHasHeader().WithData(data).Render()
	},
}

// showCmd represents the log show command
var showCmd = &cobra.Command{
	Use:   "show <transaction_id>",
	Short: "Show detailed information about a specific transaction",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := loadStateManager()
		if err != nil {
			pterm.Error.Printf("Failed to load state: %v\n", err)
			return
		}

		targetID := args[0]
		var foundTx *types.Transaction

		// Find transaction (support prefix matching)
		if manager.Current != nil {
			for _, tx := range manager.Current.History {
				if strings.HasPrefix(tx.ID, targetID) {
					foundTx = &tx
					break
				}
			}
		}

		if foundTx == nil {
			pterm.Error.Printf("Transaction '%s' not found.\n", targetID)
			return
		}

		// Display Details
		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).Printf("Transaction: %s", foundTx.ID)
		pterm.Println()
		pterm.Info.Printf("Timestamp: %s\n", foundTx.Timestamp.Format(time.RFC822))

		statusStyle := pterm.NewStyle(pterm.FgGreen)
		if foundTx.Status != "success" {
			statusStyle = pterm.NewStyle(pterm.FgRed)
		}
		pterm.Info.Printf("Status: %s\n", statusStyle.Sprint(foundTx.Status))
		pterm.Println()

		pterm.DefaultSection.Println("Changes")

		if len(foundTx.Changes) == 0 {
			pterm.Info.Println("No changes recorded in this transaction.")
			return
		}

		for _, change := range foundTx.Changes {
			pterm.Println(pterm.Bold.Sprintf("• [%s] %s (%s)", change.Type, change.Name, change.Action))
			if change.Target != "" {
				pterm.Printf("  Target: %s\n", change.Target)
			}
			if change.BackupPath != "" {
				pterm.Printf("  Backup: %s\n", change.BackupPath)
			}

			if change.Diff != "" {
				pterm.Println(pterm.FgGray.Sprint("  Diff:"))
				// Simple indentation for diff
				lines := strings.Split(change.Diff, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "+") {
						pterm.Print(pterm.FgGreen.Sprintf("    %s\n", line))
					} else if strings.HasPrefix(line, "-") {
						pterm.Print(pterm.FgRed.Sprintf("    %s\n", line))
					} else {
						pterm.Print(pterm.FgGray.Sprintf("    %s\n", line))
					}
				}
			}
			pterm.Println()
		}
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.AddCommand(showCmd)
}

func loadStateManager() (*state.Manager, error) {
	configPath := consts.GetStateFilePath()
	return state.NewManager(configPath, &core.RealFS{})
}
