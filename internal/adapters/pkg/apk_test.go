package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestApkAdapter_Check(t *testing.T) {
	tests := []struct {
		name          string
		packageName   string
		state         string
		mockRunErr    error
		expectedCheck bool
	}{
		{
			name:          "Package not installed, State=present -> Needs Action",
			packageName:   "busybox",
			state:         "present",
			mockRunErr:    errors.New("not found"), // apk info returns error if not installed
			expectedCheck: true,
		},
		{
			name:          "Package installed, State=present -> No Action",
			packageName:   "busybox",
			state:         "present",
			mockRunErr:    nil,
			expectedCheck: false,
		},
		{
			name:          "Package installed, State=absent -> Needs Action",
			packageName:   "curl",
			state:         "absent",
			mockRunErr:    nil,
			expectedCheck: true,
		},
		{
			name:          "Package not installed, State=absent -> No Action",
			packageName:   "curl",
			state:         "absent",
			mockRunErr:    errors.New("not found"),
			expectedCheck: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTr := &MockTransport{
				ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
					if !strings.HasPrefix(cmd, "apk info -e") {
						return "", fmt.Errorf("unexpected command: %s", cmd)
					}
					return "package-info", tt.mockRunErr
				},
			}

			adapter := NewApkAdapter(tt.packageName, map[string]interface{}{"state": tt.state}).(*ApkAdapter)
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

func TestApkAdapter_Apply(t *testing.T) {
	t.Run("Install success", func(t *testing.T) {
		adapter := NewApkAdapter("git", map[string]interface{}{"state": "present"}).(*ApkAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "apk info -e") {
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

		expected := "apk add git"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})

	t.Run("Remove success", func(t *testing.T) {
		adapter := NewApkAdapter("git", map[string]interface{}{"state": "absent"}).(*ApkAdapter)
		var executedCmd string

		mockTr := &MockTransport{
			ExecuteFunc: func(ctx context.Context, cmd string) (string, error) {
				if strings.HasPrefix(cmd, "apk info -e") {
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

		expected := "apk del git"
		if executedCmd != expected {
			t.Errorf("Unexpected command: got %s, want %s", executedCmd, expected)
		}
	})
}
