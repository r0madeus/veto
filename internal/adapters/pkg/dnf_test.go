package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestDnfAdapter_Check(t *testing.T) {
	tests := []struct {
		name          string
		packageName   string
		state         string
		mockRunErr    error
		expectedCheck bool
	}{
		{
			name:          "Package not installed, State=present -> Needs Action",
			packageName:   "htop",
			state:         "present",
			mockRunErr:    errors.New("not found"), // rpm -q returns error if not installed
			expectedCheck: true,
		},
		{
			name:          "Package installed, State=present -> No Action",
			packageName:   "htop",
			state:         "present",
			mockRunErr:    nil,
			expectedCheck: false,
		},
		{
			name:          "Package installed, State=absent -> Needs Action",
			packageName:   "vim",
			state:         "absent",
			mockRunErr:    nil,
			expectedCheck: true,
		},
		{
			name:          "Package not installed, State=absent -> No Action",
			packageName:   "vim",
			state:         "absent",
			mockRunErr:    errors.New("not found"),
			expectedCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTr := &MockTransport{
				ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
					if !strings.HasPrefix(cmd, "rpm -q") {
						return "", fmt.Errorf("unexpected command: %s", cmd)
					}
					return "package-version", tt.mockRunErr
				},
			}

			adapter := NewDnfAdapter(tt.packageName, map[string]interface{}{"state": tt.state}).(*DnfAdapter)
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

func TestDnfAdapter_Apply(t *testing.T) {
	t.Run("Install success", func(t *testing.T) {
		adapter := NewDnfAdapter("git", map[string]interface{}{"state": "present"}).(*DnfAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "rpm -q") {
					return "", errors.New("not installed")
				}
				executedCmd = cmd
				return "success", nil
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

		expected := "dnf install -y git"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})

	t.Run("Remove success", func(t *testing.T) {
		adapter := NewDnfAdapter("git", map[string]interface{}{"state": "absent"}).(*DnfAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "rpm -q") {
					return "installed", nil
				}
				executedCmd = cmd
				return "success", nil
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

		expected := "dnf remove -y git"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})
}
