package inventory

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Inventory represents the structure of the inventory file.
type Inventory struct {
	Hosts []Host `yaml:"hosts"`
}

// Host represents a single target machine in the fleet.
type Host struct {
	Name       string `yaml:"name"`
	Address    string `yaml:"address"`
	User       string `yaml:"user"`
	Port       int    `yaml:"port,omitempty"`
	KeyPath    string `yaml:"key_path,omitempty"`
	Connection string `yaml:"connection,omitempty"` // "ssh", "local", "winrm" (future)
	KnownHosts string `yaml:"known_hosts,omitempty"`
	// InsecureHostKey disables SSH host key verification for this host.
	InsecureHostKey bool              `yaml:"insecure_host_key,omitempty"`
	Vars            map[string]string `yaml:"vars,omitempty"`
}

// LoadInventory reads and parses the inventory file.
func LoadInventory(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read inventory file: %w", err)
	}

	var inv Inventory
	if err := yaml.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("failed to parse inventory file: %w", err)
	}

	// Set defaults
	for i := range inv.Hosts {
		if inv.Hosts[i].Port == 0 {
			inv.Hosts[i].Port = 22
		}
		if inv.Hosts[i].Connection == "" {
			if inv.Hosts[i].Address == "localhost" || inv.Hosts[i].Address == "127.0.0.1" {
				inv.Hosts[i].Connection = "local"
			} else {
				inv.Hosts[i].Connection = "ssh"
			}
		}
	}

	return &inv, nil
}
