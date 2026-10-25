package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

// MockTransport is a mock implementation of core.Transport interface.
type MockTransport struct {
	ExecuteFunc func(ctx context.Context, cmd string) (string, error)
}

func (m *MockTransport) Execute(ctx context.Context, cmd string) (string, error) {
	if m.ExecuteFunc != nil {
		return m.ExecuteFunc(ctx, cmd)
	}
	return "", nil
}

func (m *MockTransport) CopyFile(ctx context.Context, localPath, remotePath string) error { return nil }
func (m *MockTransport) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	return nil
}
func (m *MockTransport) GetFileSystem() core.FileSystem            { return &core.RealFS{} }
func (m *MockTransport) GetOS(ctx context.Context) (string, error) { return "linux", nil }
func (m *MockTransport) Close() error                              { return nil }

func TestPacmanAdapter_Check(t *testing.T) {
	// Restore original runner after tests
	defer func() { core.CommandRunner = &core.RealRunner{} }()

	tests := []struct {
		name          string
		packageName   string
		state         string
		mockRunErr    error
		expectedCheck bool
	}{
		{
			name:          "Package not installed, State=present -> Needs Action (Types.True)",
			packageName:   "git",
			state:         "present",
			mockRunErr:    errors.New("not found"), // simule "pacman -Qi" failing
			expectedCheck: true,
		},
		{
			name:          "Package installed, State=present -> No Action (Types.False)",
			packageName:   "git",
			state:         "present",
			mockRunErr:    nil, // simule "pacman -Qi" success
			expectedCheck: false,
		},
		{
			name:          "Package installed, State=absent -> Needs Action (Types.True)",
			packageName:   "vim",
			state:         "absent",
			mockRunErr:    nil,
			expectedCheck: true,
		},
		{
			name:          "Package not installed, State=absent -> No Action (Types.False)",
			packageName:   "vim",
			state:         "absent",
			mockRunErr:    errors.New("not found"),
			expectedCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Mock
			mockTr := &MockTransport{
				ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
					// Verify command is checking package existence
					if !strings.HasPrefix(cmd, "pacman -Qi") {
						return "", fmt.Errorf("unexpected command: %s", cmd)
					}
					return "installed", tt.mockRunErr
				},
			}

			adapter := NewPacmanAdapter(tt.packageName, map[string]interface{}{"state": tt.state}).(*PacmanAdapter)
			needsAction, err := adapter.Check(core.NewSystemContext(false, mockTr))

			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if needsAction != tt.expectedCheck {
				t.Errorf("Check() = %v, want %v", needsAction, tt.expectedCheck)
			}
		})
	}
}

func TestPacmanAdapter_Apply(t *testing.T) {
	defer func() { core.CommandRunner = &core.RealRunner{} }()

	t.Run("DryRun should not execute install command", func(t *testing.T) {
		adapter := NewPacmanAdapter("htop", map[string]interface{}{"state": "present"}).(*PacmanAdapter)

		// Mock check to say package is missing (so it tries to install)
		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				return "", errors.New("not installed")
			},
		}

		ctx := core.NewSystemContext(true, mockTr)
		result, err := adapter.Apply(ctx)

		if err != nil {
			t.Fatalf("Apply returned error: %v", err)
		}
		if !result.Changed {
			t.Errorf("Expected Changed=true for DryRun")
		}
		if !strings.Contains(result.Message, "DryRun") {
			t.Errorf("Expected DryRun message, got: %s", result.Message)
		}
	})

	t.Run("Install success", func(t *testing.T) {
		adapter := NewPacmanAdapter("htop", map[string]interface{}{"state": "present"}).(*PacmanAdapter)

		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				// 1. Check call (returns err -> not installed)
				if strings.Contains(cmd, "-Qi") {
					return "", errors.New("not installed")
				}
				// 2. Install call
				executedCmd = cmd
				return "installation success", nil
			},
		}

		ctx := core.NewSystemContext(false, mockTr)
		result, err := adapter.Apply(ctx)

		if err != nil {
			t.Fatalf("Apply returned error: %v", err)
		}
		if !result.Changed {
			t.Error("Expected Changed=true")
		}

		// Verify command for install: pacman -S --noconfirm --needed htop
		expected := "pacman -S --noconfirm --needed htop"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})
}

func TestPacmanAdapter_Revert(t *testing.T) {
	defer func() { core.CommandRunner = &core.RealRunner{} }()

	t.Run("Revert installed package", func(t *testing.T) {
		adapter := NewPacmanAdapter("nano", map[string]interface{}{"state": "present"}).(*PacmanAdapter)
		adapter.ActionPerformed = "installed"

		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				executedCmd = cmd
				return "removed", nil
			},
		}

		err := adapter.Revert(core.NewSystemContext(false, mockTr))
		if err != nil {
			t.Fatalf("Revert failed: %v", err)
		}

		// Verify remove command: pacman -Rns --noconfirm nano
		expected := "pacman -Rns --noconfirm nano"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})
}
