package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/r0madeus/veto/internal/consts"
)

type RecipeManager struct {
	BaseDir    string
	RecipesDir string
	ActiveFile string
}

func NewRecipeManager(baseDir string) *RecipeManager {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, consts.GetVetoDir())
	}
	return &RecipeManager{
		BaseDir:    baseDir,
		RecipesDir: filepath.Join(baseDir, consts.RecipesDirName),
		ActiveFile: filepath.Join(baseDir, "active_recipe"),
	}
}

// EnsureDirs creates base directories
func (m *RecipeManager) EnsureDirs() error {
	return os.MkdirAll(m.RecipesDir, 0755)
}

// Create creates a new recipe directory structure
// Standard Veto Recipe Structure:
// my-recipe/
// ├── system.yaml
// └── rulesets/ (directory for downloaded rulesets)
func (m *RecipeManager) Create(name string) error {
	if err := m.EnsureDirs(); err != nil {
		return err
	}

	recipeDir := filepath.Join(m.RecipesDir, name)
	if _, err := os.Stat(recipeDir); !os.IsNotExist(err) {
		return fmt.Errorf("recipe '%s' already exists", name)
	}

	// Create main dir
	if err := os.MkdirAll(recipeDir, 0755); err != nil {
		return err
	}

	// Create rulesets dir
	if err := os.MkdirAll(filepath.Join(recipeDir, "rulesets"), 0755); err != nil {
		return err
	}

	// Create system.yaml
	defaultConfig := fmt.Sprintf(`name: "%s"
version: "1.0"

# List of RuleSets to include
# rulesets:
#   - ./rulesets/docker
#   - ./rulesets/devtools

resources: []
`, name)
	configFile := filepath.Join(recipeDir, consts.SystemProfileName)

	return os.WriteFile(configFile, []byte(defaultConfig), 0644)
}

// List returns all recipe names
func (m *RecipeManager) List() ([]string, error) {
	if err := m.EnsureDirs(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.RecipesDir)
	if err != nil {
		return nil, err
	}

	var recipes []string
	for _, e := range entries {
		if e.IsDir() {
			recipes = append(recipes, e.Name())
		}
	}
	return recipes, nil
}

// Use sets the active recipe
func (m *RecipeManager) Use(name string) error {
	// Verify it exists
	recipeDir := filepath.Join(m.RecipesDir, name)
	if _, err := os.Stat(recipeDir); os.IsNotExist(err) {
		return fmt.Errorf("recipe '%s' does not exist", name)
	}

	return os.WriteFile(m.ActiveFile, []byte(name), 0644)
}

// GetActive returns the name of the active recipe
func (m *RecipeManager) GetActive() (string, error) {
	content, err := os.ReadFile(m.ActiveFile)
	if os.IsNotExist(err) {
		return "", nil // No active recipe
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// GetRecipePath returns the path to system.yaml of the given recipe (or active if empty)
func (m *RecipeManager) GetRecipePath(name string) (string, error) {
	if name == "" {
		active, err := m.GetActive()
		if err != nil {
			return "", err
		}
		if active == "" {
			return "", nil // No active recipe
		}
		name = active
	}
	return filepath.Join(m.RecipesDir, name, consts.SystemProfileName), nil
}

// GetActiveRecipeDir returns the absolute path to the active recipe directory
func (m *RecipeManager) GetActiveRecipeDir() (string, error) {
	name, err := m.GetActive()
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("no active recipe found")
	}
	return filepath.Join(m.RecipesDir, name), nil
}
