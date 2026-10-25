package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/consts"
	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/crypto"
	"github.com/r0madeus/veto/internal/system"
)

// Config represents the root structure of veto.yaml.
type Config struct {
	Vars      map[string]string `yaml:"vars,omitempty"`      // Global variables
	Variables map[string]string `yaml:"variables,omitempty"` // Global variables alias
	Includes  []string          `yaml:"includes,omitempty"`  // Other config files to include
	Imports   []string          `yaml:"imports,omitempty"`   // Alias for includes
	RuleSets  []string          `yaml:"rulesets,omitempty"`  // RuleSet paths to include
	Resources []ResourceConfig  `yaml:"resources"`           // Resource list
	Hosts     []Host            `yaml:"hosts,omitempty"`     // Remote hosts (Optional)
}

// ResourceConfig holds the configuration for each resource (file, user, package, etc.).
type ResourceConfig struct {
	ID        string                 `yaml:"id"`
	Name      string                 `yaml:"name"`
	Type      string                 `yaml:"type"`
	State     string                 `yaml:"state"`
	Priority  int                    `yaml:"priority"` // Execution priority (Higher = Earlier)
	When      string                 `yaml:"when"`     // Conditional execution logic
	DependsOn []string               `yaml:"depends_on"`
	Params    map[string]interface{} `yaml:"params"`
	Hooks     Hooks                  `yaml:"hooks"`
	Prune     bool                   `yaml:"prune"`
}

// Hooks defines lifecycle command hooks for a resource.
type Hooks struct {
	Pre      string `yaml:"pre"`       // Runs before apply
	Post     string `yaml:"post"`      // Runs after apply (always)
	OnChange string `yaml:"on_change"` // Runs only if state changed
	OnFail   string `yaml:"on_fail"`   // Runs if apply failed
}

// Host holds connection information for a remote host.
type Host struct {
	Name           string `yaml:"name"`
	Address        string `yaml:"address"`
	User           string `yaml:"user"`
	Port           int    `yaml:"port"`
	SSHKeyPath     string `yaml:"ssh_key_path"`
	BecomeMethod   string `yaml:"become_method"`   // sudo, su
	BecomePassword string `yaml:"become_password"` // Optional (Recommended to come from Vault)
}

// LoadConfig reads the YAML file at the specified path and converts it into a Config struct.
func LoadConfig(path string, decrypt bool) (*Config, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// 0. Detect System & Set VETO_ VARIABLES for Template Expansion
	// This happens BEFORE loading config so {{.OS}} works in 'includes'
	// Passing nil transport as system.Detect should handle local fallback
	ctx := core.NewSystemContext(false, nil)
	system.Detect(ctx) // Lightweight detection
	_ = os.Setenv("VETO_OS", ctx.OS)
	_ = os.Setenv("VETO_DISTRO", ctx.Distro)
	_ = os.Setenv("VETO_HOSTNAME", ctx.Hostname)

	// 1. If .env exists in project root or next to config, load it
	// Search next to config file
	baseDir := filepath.Dir(absPath)
	envPath := filepath.Join(baseDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		// .env found, load. Log if error but don't stop.
		if loadErr := godotenv.Load(envPath); loadErr != nil && !os.IsNotExist(loadErr) {
			// Just verify info, no logger yet
			fmt.Printf("Warning: Failed to load .env file: %v\n", loadErr)
		}
	} else {
		// Maybe in parent dir? (Project root)
		// Currently only checks next to config.
		// Alternative: godotenv.Load() without params searches in working dir.
		// Let's try this too:
		_ = godotenv.Load() // Ignore error (if no file found)
	}

	visited := make(map[string]bool)
	cfg, err := loadConfigRecursive(absPath, visited)
	if err != nil {
		return nil, err
	}

	// Recursive loading finished, now perform variable expansion on all string values
	expandConfig(cfg)
	if decrypt {
		decryptConfig(cfg)
	}

	return cfg, nil
}

func loadConfigRecursive(path string, visited map[string]bool) (*Config, error) {
	if visited[path] {
		return &Config{}, nil
	}
	visited[path] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file read error (%s): %w", path, err)
	}

	if len(data) == 0 {
		return &Config{}, nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("yaml parse error (%s): %w", path, err)
	}

	blockCfg := &cfg

	// Merge Variables into Vars
	if blockCfg.Variables != nil {
		if blockCfg.Vars == nil {
			blockCfg.Vars = make(map[string]string)
		}
		for k, v := range blockCfg.Variables {
			blockCfg.Vars[k] = v
		}
	}

	// Merge Imports into Includes
	blockCfg.Includes = append(blockCfg.Includes, blockCfg.Imports...)

	// Variable Expansion for path resolution before recursing
	// (If env var exists in includes field)
	for i, inc := range blockCfg.Includes {
		blockCfg.Includes[i] = os.ExpandEnv(inc)
	}

	// Process Rulesets: Treat them as includes but look for "rules.yaml" if it's a directory
	// In strict recipe mode, these are local paths.
	// We append them to Includes so the loop below handles them.
	for _, rs := range blockCfg.RuleSets {
		expandedRS := os.ExpandEnv(rs)

		// If it's a directory, assume rules.yaml inside
		// We rely on the Includes loop to resolve paths relative to the current file
		// But here we need to know if it's a dir or file to append correctly?
		// Actually, LoadConfigRecursive can handle this if we modify it, or we just append
		// the probable path.
		// Simpler approach: Check logic inside the Includes loop or pre-calculate here.
		// Since we are inside loadConfigRecursive, we don't know absolute path quite yet without check.
		// Let's modify the Includes loop to handle directories by looking for rules.yaml/main.yaml

		blockCfg.Includes = append(blockCfg.Includes, expandedRS)
	}

	// Process included files
	baseDir := filepath.Dir(path)
	var allResources []ResourceConfig

	// Include
	for _, includePath := range blockCfg.Includes {
		fullIncludePath := filepath.Join(baseDir, includePath)
		absIncludePath, err := filepath.Abs(fullIncludePath)
		if err != nil {
			return nil, err
		}

		// Directory Check
		info, err := os.Stat(absIncludePath)
		if err == nil && info.IsDir() {
			// Try rules.yaml first, then main.yaml
			rulesPath := filepath.Join(absIncludePath, "rules.yaml")
			if _, err := os.Stat(rulesPath); err == nil {
				absIncludePath = rulesPath
			} else {
				mainPath := filepath.Join(absIncludePath, "main.yaml")
				if _, err := os.Stat(mainPath); err == nil {
					absIncludePath = mainPath
				} else {
					// Directory exists but no known config file? skip or error?
					// Use filepath.Join just to fail gracefully later or log warning
					// For now, let's proceed and fail at ReadFile
					fmt.Printf("Warning: Included directory '%s' has no rules.yaml or main.yaml\n", includePath)
				}
			}
		}

		subCfg, err := loadConfigRecursive(absIncludePath, visited)
		if err != nil {
			return nil, err
		}

		allResources = append(allResources, subCfg.Resources...)

		if blockCfg.Vars == nil {
			blockCfg.Vars = make(map[string]string)
		}
		for k, v := range subCfg.Vars {
			if _, exists := blockCfg.Vars[k]; !exists {
				blockCfg.Vars[k] = v
			}
		}
	}

	allResources = append(allResources, blockCfg.Resources...)
	blockCfg.Resources = allResources

	return blockCfg, nil
}

// expandConfig performs Env Var substitution on all string values in the configuration.
func expandConfig(cfg *Config) {
	// 1. Global Vars
	for k, v := range cfg.Vars {
		expanded := os.ExpandEnv(v)
		cfg.Vars[k] = expanded
		// Add to environment so resources can use these variables
		_ = os.Setenv(k, expanded)
	}

	// 2. Resources
	for i := range cfg.Resources {
		expandResource(&cfg.Resources[i])
	}

	// 3. Hosts
	for i := range cfg.Hosts {
		cfg.Hosts[i].Address = os.ExpandEnv(cfg.Hosts[i].Address)
		cfg.Hosts[i].User = os.ExpandEnv(cfg.Hosts[i].User)
		cfg.Hosts[i].BecomePassword = os.ExpandEnv(cfg.Hosts[i].BecomePassword)
		cfg.Hosts[i].SSHKeyPath = os.ExpandEnv(cfg.Hosts[i].SSHKeyPath)
	}
}

func expandResource(res *ResourceConfig) {
	res.Name = os.ExpandEnv(res.Name)

	// Set default ID if empty
	if res.ID == "" {
		res.ID = fmt.Sprintf("%s:%s", res.Type, res.Name)
	}
	// ID does not change, references might break.
	// Type and State can also be expanded
	res.Type = os.ExpandEnv(res.Type)
	res.State = os.ExpandEnv(res.State)

	// Params (Recursive Map traversal)
	expandMap(res.Params)
}

func expandMap(m map[string]interface{}) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			m[k] = os.ExpandEnv(val)
		case map[string]interface{}:
			expandMap(val)
		case []interface{}:
			for i, item := range val {
				if str, ok := item.(string); ok {
					val[i] = os.ExpandEnv(str)
				} else if subMap, ok := item.(map[string]interface{}); ok {
					expandMap(subMap)
				}
			}
		}
	}
}

// Security & Decryption

func decryptConfig(cfg *Config) {
	if !hasEncryptedContent(cfg) {
		return
	}

	key := getMasterKey()
	if key == "" {
		// No key found. If there are encrypted values, they will remain as is (encrypted).
		// This might cause errors later if the app expects plaintext, but safer than crashing.
		// Alternatively, we could scan for IsEncrypted and warn.
		return
	}

	// 1. Global Vars
	for k, v := range cfg.Vars {
		if decrypted, err := crypto.Decrypt(v, key); err == nil {
			cfg.Vars[k] = decrypted
			_ = os.Setenv(k, decrypted)
		}
	}

	// 2. Resources
	for i := range cfg.Resources {
		decryptResource(&cfg.Resources[i], key)
	}

	// 3. Hosts
	for i := range cfg.Hosts {
		if val, err := crypto.Decrypt(cfg.Hosts[i].BecomePassword, key); err == nil {
			cfg.Hosts[i].BecomePassword = val
		}
	}
}

func hasEncryptedContent(cfg *Config) bool {
	// 1. Global Vars
	for _, v := range cfg.Vars {
		if crypto.IsEncrypted(v) {
			return true
		}
	}

	// 2. Resources
	for i := range cfg.Resources {
		if hasEncryptedResource(&cfg.Resources[i]) {
			return true
		}
	}

	// 3. Hosts
	for _, h := range cfg.Hosts {
		if crypto.IsEncrypted(h.BecomePassword) {
			return true
		}
	}

	return false
}

func hasEncryptedResource(res *ResourceConfig) bool {
	return hasEncryptedMap(res.Params)
}

func hasEncryptedMap(m map[string]interface{}) bool {
	for _, v := range m {
		switch val := v.(type) {
		case string:
			if crypto.IsEncrypted(val) {
				return true
			}
		case map[string]interface{}:
			if hasEncryptedMap(val) {
				return true
			}
		case []interface{}:
			for _, item := range val {
				if str, ok := item.(string); ok {
					if crypto.IsEncrypted(str) {
						return true
					}
				} else if subMap, ok := item.(map[string]interface{}); ok {
					if hasEncryptedMap(subMap) {
						return true
					}
				}
			}
		}
	}
	return false
}

func decryptResource(res *ResourceConfig, key string) {
	// Params (Recursive Map traversal)
	decryptMap(res.Params, key)
}

func decryptMap(m map[string]interface{}, key string) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if crypto.IsEncrypted(val) {
				if decrypted, err := crypto.Decrypt(val, key); err == nil {
					m[k] = decrypted
				}
			}
		case map[string]interface{}:
			decryptMap(val, key)
		case []interface{}:
			for i, item := range val {
				if str, ok := item.(string); ok {
					if crypto.IsEncrypted(str) {
						if decrypted, err := crypto.Decrypt(str, key); err == nil {
							val[i] = decrypted
						}
					}
				} else if subMap, ok := item.(map[string]interface{}); ok {
					decryptMap(subMap, key)
				}
			}
		}
	}
}

func getMasterKey() string {
	// 1. Env Var
	if key := os.Getenv("VETO_MASTER_KEY"); key != "" {
		return key
	}

	// 2. File
	if keyPath, err := consts.GetMasterKeyPath(); err == nil {
		if content, err := os.ReadFile(keyPath); err == nil {
			return string(content)
		}
	}

	// 3. Interactive Prompt
	// Only if stdin is a terminal (interactive session)
	if isInteractive() {
		// Use pterm to ask for password
		pterm.Println()
		pterm.Warning.Println("Encrypted content detected but VETO_MASTER_KEY not found.")
		key, err := pterm.DefaultInteractiveTextInput.
			WithMask("*").
			WithDefaultText("Enter Master Key for decryption").
			Show()

		if err == nil && key != "" {
			return key
		}
	}

	return ""
}

func isInteractive() bool {
	fileInfo, _ := os.Stdin.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
