package cmd

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/resource"
	"github.com/r0madeus/veto/internal/state"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/r0madeus/veto/internal/types"
	"github.com/spf13/cobra"
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback [count]",
	Short: "Rollback the last N transactions",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		count := 1
		if len(args) > 0 {
			fmt.Sscanf(args[0], "%d", &count)
		}

		// Initialize State Manager
		fs := core.RealFS{}
		mgr, err := state.NewManager(consts.GetStateFilePath(), &fs) // Use default path
		if err != nil {
			pterm.Error.Printf("Failed to load state: %v\n", err)
			return
		}

		txs := mgr.GetTransactions()
		if len(txs) == 0 {
			pterm.Info.Println("No history to rollback.")
			return
		}

		// Determine transactions to rollback (LIFO)
		if count > len(txs) {
			count = len(txs)
		}

		// Get last N transactions, reversed
		toRollback := make([]types.Transaction, 0, count)
		for i := len(txs) - 1; i >= len(txs)-count; i-- {
			toRollback = append(toRollback, txs[i])
		}

		pterm.DefaultHeader.Printf("Rolling Back %d Transactions", count)

		// Initialize System Context
		ctx := core.NewSystemContext(false, transport.NewLocalTransport())
		system.Detect(ctx)

		// Execute Rollback
		for _, tx := range toRollback {
			pterm.Info.Printf("Rolling back transaction: %s\n", tx.ID)

			// Revert changes in reverse order within the transaction
			for i := len(tx.Changes) - 1; i >= 0; i-- {
				change := tx.Changes[i]
				pterm.Info.Printf("  Reverting: %s %s (%s)\n", change.Action, change.Name, change.Type)

				if err := performRollback(change, ctx); err != nil {
					pterm.Error.Printf("Failed to revert: %v\n", err)
				} else {
					pterm.Success.Println("Reverted.")
				}
			}
		}
	},
}

func performRollback(change types.TransactionChange, ctx *core.SystemContext) error {
	// 1. Identify Resource
	resType := change.Type
	resName := change.Name

	if resType == "" || resName == "" {
		return fmt.Errorf("invalid resource info")
	}

	// 2. Create Resource Instance
	params := make(map[string]interface{})
	if change.BackupPath != "" {
		params["backup_path"] = change.BackupPath
	}

	res, err := resource.CreateResourceWithParams(resType, resName, params, ctx)
	if err != nil {
		return fmt.Errorf("failed to create resource factory: %w", err)
	}

	// 3. Cast to Revertable
	revertable, ok := res.(core.Revertable)
	if !ok {
		return fmt.Errorf("resource %s does not support rollback", resType)
	}

	// 4. Execute RevertAction
	return revertable.RevertAction(change.Action, ctx)
}

func init() {
	rootCmd.AddCommand(rollbackCmd)
}
