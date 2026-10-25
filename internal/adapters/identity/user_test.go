package identity

import (
	"context"
	"testing"

	"github.com/r0madeus/veto/internal/core"
	"github.com/r0madeus/veto/internal/transport"
	"github.com/stretchr/testify/assert"
)

func TestUserApply_Create(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
	}

	// 1. Check fails (user missing)
	mockTr.AddError("getent passwd testuser", assert.AnError)
	mockTr.AddError("id -u testuser", assert.AnError) // For useradd decision

	// 2. Apply calls useradd
	mockTr.AddResponse("useradd -u 2001 -d /home/testuser -m -s /bin/bash testuser", "")

	// 3. Command execution for check is separate
	mockTr.AddError("getent passwd testuser", assert.AnError)

	res := NewUserAdapter("testuser", map[string]interface{}{
		"uid":   "2001",
		"shell": "/bin/bash",
		"home":  "/home/testuser", // Provided
		"state": "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Message, "User created")
}

func TestUserApply_Modify(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
	}

	// 1. Check finds drift (UID mismatch)
	// Current: uid=1000
	mockTr.AddResponse("getent passwd testuser", "testuser:x:1000:1000::/home/testuser:/bin/bash")

	// 2. Apply detects user exists
	mockTr.AddResponse("id -u testuser", "1000") // Exists

	// 3. Apply calls usermod
	mockTr.AddResponse("usermod -u 2001 testuser", "")

	res := NewUserAdapter("testuser", map[string]interface{}{
		"uid":   "2001", // Desired: 2001
		"state": "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Message, "User modified")
}

func TestUserApply_NoChange(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
	}

	// 1. Check matches
	mockTr.AddResponse("getent passwd testuser", "testuser:x:2001:2001::/home/testuser:/bin/bash")

	res := NewUserAdapter("testuser", map[string]interface{}{
		"uid":   "2001",
		"shell": "/bin/bash",
		"home":  "/home/testuser",
		"state": "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.False(t, result.Changed)
}

func TestGroupApply_Create(t *testing.T) {
	mockTr := transport.NewMockTransport()
	ctx := &core.SystemContext{
		Context:   context.Background(),
		Transport: mockTr,
	}

	// 1. Check fails
	mockTr.AddError("getent group testgroup", assert.AnError)

	// 2. Apply calls groupadd
	mockTr.AddResponse("groupadd -g 2001 testgroup", "")

	res := NewGroupAdapter("testgroup", map[string]interface{}{
		"gid":   2001,
		"state": "present",
	})

	result, err := res.Apply(ctx)

	assert.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Message, "Group created")
}
