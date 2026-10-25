package hub

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
)

// HubClient interacts with the Veto Registry via Git
type HubClient struct {
	RegistryURL string
	LocalPath   string
}

// NewHubClient creates a new client pointing to the official recipe repo
func NewHubClient(localPath string) *HubClient {
	if localPath == "" {
		localPath, _ = consts.GetHubIndexPath()
	}
	return &HubClient{
		// Defaults to a placeholder generic repo or official one if it existed.
		// For now, we can allow overriding or default to a safe example.
		// Since this is a "Project Agnostic" tool, maybe valid to keep it configurable?
		// Setting a default placeholders.
		RegistryURL: consts.DefaultHubRepo,
		LocalPath:   localPath,
	}
}

// Update syncs the local registry with the remote git repository
func (c *HubClient) Update() error {
	// Check if .git exists
	gitDir := filepath.Join(c.LocalPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		// Update existing
		pterm.Info.Println("Updating registry...")
		cmd := exec.Command("git", "-C", c.LocalPath, "pull", "--rebase")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Clone new
	_ = os.MkdirAll(filepath.Dir(c.LocalPath), 0755) // Ensure parent exists
	pterm.Info.Printf("Cloning registry from %s...\n", c.RegistryURL)
	cmd := exec.Command("git", "clone", c.RegistryURL, c.LocalPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Search returns a list of available recipes matching the query
func (c *HubClient) Search(query string) ([]string, error) {
	// Walk the directory structure
	// Structure: <category>/<recipe-name>/system.yaml
	// OR flat: <recipe-name>/system.yaml

	var results []string

	// Ensure index exists
	if _, err := os.Stat(c.LocalPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("registry not initialized. Run 'veto hub update' first")
	}

	err := filepath.WalkDir(c.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Look for system.yaml to identify a valid recipe
		if !d.IsDir() && d.Name() == consts.SystemProfileName {
			// Get relative path from hub root
			relPath, _ := filepath.Rel(c.LocalPath, filepath.Dir(path))

			// Simple search match
			if query == "" || strings.Contains(strings.ToLower(relPath), strings.ToLower(query)) {
				results = append(results, relPath)
			}
		}

		return nil
	})

	return results, err
}

// Install copies a recipe from the hub to the user's recipes directory
func (c *HubClient) Install(recipeName, destRecipeDir string) error {
	srcDir := filepath.Join(c.LocalPath, recipeName)

	// Verify source exists
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return fmt.Errorf("recipe '%s' not found in registry", recipeName)
	}

	// Verify dest doesn't exist
	if _, err := os.Stat(destRecipeDir); !os.IsNotExist(err) {
		return fmt.Errorf("destination '%s' already exists", destRecipeDir)
	}

	// Recursive Copy
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destRecipeDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy file content
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destPath, data, info.Mode())
	})
}
