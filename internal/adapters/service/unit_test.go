package service

import (
	"context"
	"testing"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/stretchr/testify/assert"
)

func TestUnitApply_Create(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
		FS:        mockTr.GetFileSystem(), // Use mock FS
	}

	unitContent := "[Unit]\nDescription=Test\n"

	// 1. Check (File missing in MockFS initially)

	// 2. Apply writes file
	// FS is in-memory, so WriteFile will succeed.

	// 3. Apply calls daemon-reload
	mockTr.AddResponse("systemctl daemon-reload", "")

	res := NewSystemdUnitAdapter("test.service", map[string]interface{}{
		"content": unitContent,
		"state":   "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Message, "Unit file created")

	// Verify file content
	savedContent, err := ctx.FS.ReadFile("/etc/systemd/system/test.service")
	assert.NoError(t, err)
	assert.Equal(t, unitContent, string(savedContent))
}

func TestUnitApply_NoChange(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
		FS:        mockTr.GetFileSystem(),
	}

	unitContent := "[Unit]\nDescription=Test\n"

	// Pre-create file
	if err := ctx.FS.WriteFile("/etc/systemd/system/test.service", []byte(unitContent), 0644); err != nil {
		t.Fatal(err)
	}

	res := NewSystemdUnitAdapter("test.service", map[string]interface{}{
		"content": unitContent,
		"state":   "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.False(t, result.Changed)
}
