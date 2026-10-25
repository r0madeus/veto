package docker

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestPodmanAdapter_Check_Running(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock Inspect Output
	inspectData := []InspectResult{
		{
			State: struct {
				Running bool
				Status  string
			}{Running: true, Status: "running"},
			Config: struct{ Image string }{Image: "nginx:latest"},
			Image:  "sha256:12345",
		},
	}
	inspectJSON, _ := json.Marshal(inspectData)
	mockTransport.OnExecute("podman inspect my-pod", string(inspectJSON), nil)

	params := map[string]interface{}{
		"image": "nginx:latest",
		"state": "running",
	}

	adapter, _ := NewPodmanAdapter("my-pod", params, ctx)

	needsAction, err := adapter.(*ContainerAdapter).Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if needsAction {
		t.Fatal("Expected needsAction=false for running container")
	}
}

func TestPodmanAdapter_Apply_Create(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// 1. Inspect (Not Found)
	mockTransport.OnExecute("podman inspect my-pod", "", &os.PathError{Err: os.ErrNotExist})

	// 2. Run
	expectedRun := "podman run -d --name my-pod nginx:latest"
	mockTransport.OnExecute(expectedRun, "container-id", nil)

	params := map[string]interface{}{
		"image": "nginx:latest",
		"state": "running",
	}

	adapter, _ := NewPodmanAdapter("my-pod", params, ctx)

	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true")
	}

	if !mockTransport.AssertCalled("podman run") {
		t.Fatal("Podman run was not called")
	}
}
