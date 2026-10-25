package consts

import (
	"os"
	"path/filepath"
)

// Constants for configuration paths and defaults
const (
	DefaultDirName    = ".veto"
	StateFileName     = "state.json"
	SystemProfileName = "system.yaml"
	IgnoreFileName    = ".vetoignore"
	MasterKeyFileName = "master.key"
	BackupDirName     = "backups"
	HubDirName        = "hub"
	HubIndexDir       = "index"
	RecipesDirName    = "recipes"
	FilesDirName      = "files"
	DefaultHubRepo    = "https://github.com/r0madeus/veto-recipes.git"
)

// GetVetoDir returns the root directory name for Veto configuration
func GetVetoDir() string {
	if env := os.Getenv("VETO_HOME"); env != "" {
		return env
	}
	return DefaultDirName
}

// GetStateFilePath returns the path to the state file
func GetStateFilePath() string {
	return filepath.Join(GetVetoDir(), StateFileName)
}

// GetSystemProfilePath returns the path to the system profile file
func GetSystemProfilePath() string {
	return filepath.Join(GetVetoDir(), SystemProfileName)
}

// GetIgnoreFilePath returns the path to the ignore file
func GetIgnoreFilePath() string {
	return IgnoreFileName
}

// GetMasterKeyPath returns the default path for the master key (user home aware)
func GetMasterKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultDirName, MasterKeyFileName), nil
}

// GetHubIndexPath returns the path where the hub index is stored
func GetHubIndexPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultDirName, HubDirName, HubIndexDir), nil
}

// GetRecipesPath returns the path where recipes are installed
func GetRecipesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultDirName, RecipesDirName), nil
}
