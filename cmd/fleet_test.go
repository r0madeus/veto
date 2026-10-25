package cmd

import (
	"context"
	"testing"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/system"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/stretchr/testify/assert"
)

func TestFleetFacts(t *testing.T) {
	// Setup Mock Transport
	mock := transport.NewMockTransport()
	mock.AddResponse("hostname", "mock-server")
	mock.AddResponse("uname -s", "Linux")
	mock.AddResponse("uname -r", "5.15.0-mock")
	mock.AddResponse("nproc", "4")
	mock.AddResponse("id -u -n", "testuser")
	mock.AddResponse("id -u", "1000")
	mock.AddResponse("id -g", "1000")
	mock.AddResponse("echo $HOME", "/home/testuser")
	mock.AddResponse("echo $SHELL", "/bin/bash")
	mock.AddResponse("echo $LANG", "en_US.UTF-8")
	mock.AddResponse("echo $TERM", "xterm")
	mock.AddResponse("lspci", "00:02.0 VGA compatible controller: Intel Corporation UHD Graphics")

	// Mock file content for OS release
	mock.FileContent["/etc/os-release"] = `ID=ubuntu
VERSION_ID="22.04"
ID_LIKE=debian
PRETTY_NAME="Ubuntu 22.04 LTS"
`
	// Mock file content for meminfo
	mock.FileContent["/proc/meminfo"] = `MemTotal:        8000000 kB
MemFree:         1000000 kB
`

	// Context with Mock Transport
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mock,
		FS:        mock.GetFileSystem(), // Important: Inject FS for readOSRelease
	}

	// Execute Detection
	system.Detect(ctx)

	// Assertions
	assert.Equal(t, "mock-server", ctx.Hostname)
	assert.Equal(t, "linux", ctx.OS)
	assert.Equal(t, "ubuntu", ctx.Distro)
	assert.Equal(t, "22.04", ctx.Version)
	assert.Equal(t, "5.15.0-mock", ctx.Kernel)
	assert.Equal(t, 4, ctx.Hardware.CPUCore)
	assert.Equal(t, "testuser", ctx.User)
	assert.Equal(t, "Intel", ctx.Hardware.GPUVendor)

	// Verify the mocked transport was actually used
	// (Implicit by the fact we got "mock-server" instead of real hostname)
}

func TestFleetFactsCommand(t *testing.T) {
	// Ideally we would invoke the Cobra command here.
	// But `fleet facts` relies on `inventory.LoadInventory` which reads a file.
	// And it creates SSHTransport internally in the loop.
	// To test the *command* flow, we would need to dependency-inject the Transport factory into Fleet command.
	// For now, testing `system.Detect` with MockTransport (above) covers the core logic.
	// The Command wrapper just iterates and prints table.
}
