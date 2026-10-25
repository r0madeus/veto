package resource

import (
	"fmt"
	"testing"

	"github.com/r0madeus/veto/internal/adapters/service"
	"github.com/r0madeus/veto/internal/core"
)

func TestCreateResourceWithParams(t *testing.T) {
	baseCtx := core.NewSystemContext(false, nil)

	tests := []struct {
		name        string
		resType     string
		resName     string
		params      map[string]interface{}
		ctxOverride func(*core.SystemContext)
		wantErr     bool
	}{
		// YENİ: User & Group
		{
			name:    "Create User Resource",
			resType: "user",
			resName: "testuser",
			params: map[string]interface{}{
				"uid":    "1001",
				"shell":  "/bin/zsh",
				"groups": []string{"wheel", "docker"},
			},
			wantErr: false,
		},
		{
			name:    "Create Group Resource",
			resType: "group",
			resName: "docker",
			params: map[string]interface{}{
				"gid":    999,
				"system": true,
			},
			wantErr: false,
		},
		// YENİ: Template & LineInFile
		{
			name:    "Create Template Resource",
			resType: "template",
			resName: "/etc/config.conf",
			params: map[string]interface{}{
				"src":  "./templates/config.tmpl",
				"vars": map[string]interface{}{"Port": 8080},
			},
			wantErr: false,
		},
		{
			name:    "Create LineInFile Resource",
			resType: "line_in_file",
			resName: "/etc/hosts",
			params: map[string]interface{}{
				"line":   "127.0.0.1 localhost",
				"regexp": "^127\\.0\\.0\\.1",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localCtx := *baseCtx
			if tt.ctxOverride != nil {
				tt.ctxOverride(&localCtx)
			}
			res, err := CreateResourceWithParams(tt.resType, tt.resName, tt.params, &localCtx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && res == nil {
				t.Errorf("Returned nil resource")
			}
		})
	}
}

func TestCreateResource_ContextAware(t *testing.T) {
	// Mock IsCommandAvailable to return false for fallbacks
	oldAvailable := core.IsCommandAvailable
	core.IsCommandAvailable = func(name string) bool { return false }
	defer func() { core.IsCommandAvailable = oldAvailable }()

	tests := []struct {
		distro   string
		wantType string
	}{
		{"arch", "*pkg.PacmanAdapter"},
		{"cachyos", "*pkg.PacmanAdapter"},
		{"manjaro", "*pkg.PacmanAdapter"},
		{"ubuntu", "*pkg.AptAdapter"},
		{"debian", "*pkg.AptAdapter"},
		{"fedora", "*pkg.DnfAdapter"},
		{"alpine", "*pkg.ApkAdapter"},
		{"opensuse", "*pkg.ZypperAdapter"},
		// Unknown falls back to error
		{"unknown-os", ""},
	}

	for _, tt := range tests {
		t.Run("Distro="+tt.distro, func(t *testing.T) {
			ctx := core.NewSystemContext(false, nil)
			ctx.Distro = tt.distro

			// Request generic "pkg" resource
			res, err := CreateResourceWithParams("pkg", "vim", nil, ctx)

			if tt.wantType == "" {
				if err == nil {
					t.Errorf("Expected error for distro %s, got none", tt.distro)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				gotType := fmt.Sprintf("%T", res)
				if gotType != tt.wantType {
					t.Errorf("For distro %s, want type %s, got %s", tt.distro, tt.wantType, gotType)
				}
			}
		})
	}
}

func TestCreateResource_InitSystemAware(t *testing.T) {
	tests := []struct {
		initSystem  string
		wantManager string // Type name of the inner manager
	}{
		{"systemd", "*service.SystemdManager"},
		{"openrc", "*service.OpenRCManager"},
		{"sysvinit", "*service.SysVinitManager"},
		// Default fallback
		{"unknown", "*service.SystemdManager"},
	}

	for _, tt := range tests {
		t.Run("Init="+tt.initSystem, func(t *testing.T) {
			ctx := core.NewSystemContext(false, nil)
			ctx.InitSystem = tt.initSystem

			// Request "service" resource
			res, err := CreateResourceWithParams("service", "nginx", nil, ctx)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// ServiceAdapter struct'ı içindeki Manager alanını kontrol etmeliyiz.
			// ServiceAdapter public olmadığı için cast edemeyebiliriz ama tipini kontrol edebiliriz.
			// Ancak burada 'factory' paketi 'service' paketini import ediyor.
			// ServiceAdapter export edilmiş durumda.

			svcAdapter, ok := res.(*service.ServiceAdapter)
			if !ok {
				t.Fatalf("Expected *service.ServiceAdapter, got %T", res)
			}

			gotManager := fmt.Sprintf("%T", svcAdapter.Manager)
			if gotManager != tt.wantManager {
				t.Errorf("For init %s, want manager %s, got %s", tt.initSystem, tt.wantManager, gotManager)
			}
		})
	}
}
