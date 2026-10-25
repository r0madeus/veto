package discovery

import (
	"fmt"

	"github.com/r0madeus/veto/internal/config"
	"github.com/r0madeus/veto/internal/core"
)

// DiscoverPackages scans for packages
func DiscoverPackages(ctx *core.SystemContext) ([]string, error) {
	return discoverPackages(ctx)
}

// DiscoverServices scans for services
func DiscoverServices(ctx *core.SystemContext) ([]string, error) {
	return discoverServices(ctx)
}

// DiscoverSystem scans the system and returns a generated configuration.
// Deprecated: logic moved to cmd/import.go for interactivity
func DiscoverSystem(ctx *core.SystemContext) (*config.Config, error) {
	cfg := &config.Config{
		Resources: []config.ResourceConfig{},
	}

	// 1. Discover Packages
	pkgs, err := DiscoverPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("package discovery failed: %w", err)
	}

	for _, pkgName := range pkgs {
		cfg.Resources = append(cfg.Resources, config.ResourceConfig{
			Type:  "pkg",
			Name:  pkgName,
			State: "present",
		})
	}

	// 2. Discover Services
	services, err := DiscoverServices(ctx)
	if err != nil {
		fmt.Printf("Warning: Service discovery failed: %v\n", err)
	}

	for _, svcName := range services {
		cfg.Resources = append(cfg.Resources, config.ResourceConfig{
			Type:  "service",
			Name:  svcName,
			State: "running", // or enabled
			Params: map[string]interface{}{
				"enabled": true,
			},
		})
	}

	return cfg, nil
}
