package network

import (
	"os"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestFirewall_Check_Exists(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock UFW Status output showing rule exists
	mockTransport.OnExecute("ufw status numbered", "[ 1] 80/tcp                   ALLOW IN    Anywhere", nil)

	params := map[string]interface{}{
		"port":  80,
		"proto": "tcp",
		"state": "present",
	}

	adapter := NewFirewallAdapter("http-rule", params)
	needs, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if needs {
		t.Fatal("Expected needs=false (rule exists)")
	}
}

func TestFirewall_Check_NotExists(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	mockTransport.OnExecute("ufw status numbered", "Status: active\n\n     To                         Action      From\n     --                         ------      ----", nil)

	params := map[string]interface{}{
		"port":  80,
		"proto": "tcp",
		"state": "present",
	}

	adapter := NewFirewallAdapter("http-rule", params)
	needs, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !needs {
		t.Fatal("Expected needs=true (rule missing)")
	}
}

func TestFirewall_Apply_Allow(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// First output (Check): empty
	mockTransport.OnExecute("ufw status numbered", "", nil)

	// Second output (Apply): success
	mockTransport.OnExecute("ufw allow 80/tcp", "Rule added", nil)

	params := map[string]interface{}{
		"port":  80,
		"proto": "tcp",
		"state": "present",
	}

	adapter := NewFirewallAdapter("http-rule", params)
	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true")
	}

	if !mockTransport.AssertCalled("ufw allow 80/tcp") {
		t.Fatal("Expected command not called")
	}
}

func TestFirewall_Apply_Delete(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// First output (Check): exists
	mockTransport.OnExecute("ufw status numbered", "[ 1] 80/tcp                   ALLOW IN    Anywhere", nil)

	mockTransport.OnExecute("ufw delete allow 80/tcp", "Rule deleted", nil)

	params := map[string]interface{}{
		"port":  80,
		"proto": "tcp",
		"state": "absent",
	}

	adapter := NewFirewallAdapter("http-rule", params)
	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true")
	}

	if !mockTransport.AssertCalled("ufw delete allow 80/tcp") {
		t.Fatal("Expected command not called")
	}
}
