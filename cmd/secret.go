package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/crypto"
	"github.com/spf13/cobra"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage encrypted secrets",
	Long:  `Utilities for generating keys and encrypting/decrypting sensitive values.`,
}

var keygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate a new master key",
	Run: func(cmd *cobra.Command, args []string) {
		key, err := crypto.GenerateKey()
		if err != nil {
			pterm.Error.Println("Failed to generate key:", err)
			return
		}
		pterm.Success.Println("Generated Master Key:")
		fmt.Println(key)
		pterm.Info.Printf("Save this key to ~/%s/%s or set VETO_MASTER_KEY environment variable.\n", consts.DefaultDirName, consts.MasterKeyFileName)
	},
}

var encryptCmd = &cobra.Command{
	Use:   "encrypt [value]",
	Short: "Encrypt a value",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := getMasterKey()
		if key == "" {
			return
		}

		encrypted, err := crypto.Encrypt(args[0], key)
		if err != nil {
			pterm.Error.Println("Encryption failed:", err)
			return
		}

		pterm.Success.Println("Encrypted Value:")
		fmt.Println(encrypted)
	},
}

var decryptCmd = &cobra.Command{
	Use:   "decrypt [encrypted_value]",
	Short: "Decrypt a value",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := getMasterKey()
		if key == "" {
			return
		}

		decrypted, err := crypto.Decrypt(args[0], key)
		if err != nil {
			pterm.Error.Println("Decryption failed:", err)
			return
		}

		pterm.Success.Println("Decrypted Value:")
		fmt.Println(decrypted)
	},
}

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(keygenCmd)
	secretCmd.AddCommand(encryptCmd)
	secretCmd.AddCommand(decryptCmd)
}

func getMasterKey() string {
	// 1. Env Var
	if key := os.Getenv("VETO_MASTER_KEY"); key != "" {
		return strings.TrimSpace(key)
	}

	// 2. File
	if keyPath, err := consts.GetMasterKeyPath(); err == nil {
		if content, err := os.ReadFile(keyPath); err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	pterm.Error.Println("Master Key not found!")
	pterm.Info.Printf("Please set VETO_MASTER_KEY or create %s\n", consts.MasterKeyFileName)
	return ""
}
