package docker

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

func TestDockerAdapter_Check_Running(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock Inspect Output (Running)
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
	mockTransport.OnExecute("docker inspect my-nginx", string(inspectJSON), nil)

	params := map[string]interface{}{
		"image": "nginx:latest",
		"state": "running",
	}

	adapter, _ := NewDockerAdapter("my-nginx", params, ctx)

	needsAction, err := adapter.(*ContainerAdapter).Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if needsAction {
		t.Fatal("Expected needsAction=false for running container")
	}
}

func TestDockerAdapter_Check_Stopped(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock Inspect Output (Stopped)
	inspectData := []InspectResult{
		{
			State: struct {
				Running bool
				Status  string
			}{Running: false, Status: "exited"},
			Config: struct{ Image string }{Image: "nginx:latest"},
			Image:  "sha256:12345",
		},
	}
	inspectJSON, _ := json.Marshal(inspectData)
	mockTransport.OnExecute("docker inspect my-nginx", string(inspectJSON), nil)

	params := map[string]interface{}{
		"image": "nginx:latest",
		"state": "running", // We want it running
	}

	adapter, _ := NewDockerAdapter("my-nginx", params, ctx)

	needsAction, err := adapter.(*ContainerAdapter).Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !needsAction {
		t.Fatal("Expected needsAction=true for stopped container when desired is running")
	}
}

func TestDockerAdapter_Apply_Create(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock Inspect (Not Found)
	mockTransport.OnExecute("docker inspect my-nginx", "", &os.PathError{Err: os.ErrNotExist}) // Simulate missing

	// Mock Run
	expectedRun := "docker run -d --name my-nginx nginx:latest"
	mockTransport.OnExecute(expectedRun, "container-id", nil)

	params := map[string]interface{}{
		"image": "nginx:latest",
		"state": "running",
	}

	adapter, _ := NewDockerAdapter("my-nginx", params, ctx)

	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true")
	}

	if !mockTransport.AssertCalled("docker run") {
		t.Fatal("Docker run was not called")
	}
}
