package shell

import (
	"errors"
	"os"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestExecAdapter_Apply(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	mockTransport.OnExecute("echo hello", "hello", nil)

	params := map[string]interface{}{
		"command": "echo hello",
	}

	adapter := NewExecAdapter("test-exec", params)

	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true for exec")
	}

	if !mockTransport.AssertCalled("echo hello") {
		t.Fatal("Command was not executed")
	}
}

func TestExecAdapter_Check_Unless(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Case 1: Unless command succeeds (exit 0) -> Should Skip
	mockTransport.OnExecute("test -f /tmp/lock", "", nil)

	params := map[string]interface{}{
		"command": "do something",
		"unless":  "test -f /tmp/lock",
	}

	adapter := NewExecAdapter("test-unless-skip", params)

	needsAction, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if needsAction {
		t.Fatal("Expected needsAction=false when unless succeeds")
	}
}

func TestExecAdapter_Check_Unless_Fail(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Case 2: Unless command fails (exit 1) -> Should Run
	mockTransport.OnExecute("test -f /tmp/lock", "", errors.New("exit status 1"))

	params := map[string]interface{}{
		"command": "do something",
		"unless":  "test -f /tmp/lock",
	}

	adapter := NewExecAdapter("test-unless-run", params)

	needsAction, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !needsAction {
		t.Fatal("Expected needsAction=true when unless fails")
	}
}
