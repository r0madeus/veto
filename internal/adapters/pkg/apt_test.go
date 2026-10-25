package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestAptAdapter_Check(t *testing.T) {
	tests := []struct {
		name          string
		packageName   string
		state         string
		mockRunErr    error
		expectedCheck bool
	}{
		{
			name:          "Package not installed, State=present -> Needs Action",
			packageName:   "curl",
			state:         "present",
			mockRunErr:    errors.New("not found"), // dpkg -s returns error if not installed
			expectedCheck: true,
		},
		{
			name:          "Package installed, State=present -> No Action",
			packageName:   "curl",
			state:         "present",
			mockRunErr:    nil,
			expectedCheck: false,
		},
		{
			name:          "Package installed, State=absent -> Needs Action",
			packageName:   "wget",
			state:         "absent",
			mockRunErr:    nil,
			expectedCheck: true,
		},
		{
			name:          "Package not installed, State=absent -> No Action",
			packageName:   "wget",
			state:         "absent",
			mockRunErr:    errors.New("not found"),
			expectedCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTr := &MockTransport{
				ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
					if !strings.HasPrefix(cmd, "dpkg -s") {
						return "", fmt.Errorf("unexpected command: %s", cmd)
					}
					return "status", tt.mockRunErr
				},
			}

			adapter := NewAptAdapter(tt.packageName, map[string]interface{}{"state": tt.state}).(*AptAdapter)
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

func TestAptAdapter_Apply(t *testing.T) {
	t.Run("Install success", func(t *testing.T) {
		adapter := NewAptAdapter("jq", map[string]interface{}{"state": "present"}).(*AptAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "dpkg -s") {
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

		expected := "apt-get install -y jq"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})

	t.Run("Remove success", func(t *testing.T) {
		adapter := NewAptAdapter("jq", map[string]interface{}{"state": "absent"}).(*AptAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "dpkg -s") {
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

		expected := "apt-get remove -y jq"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})
}
