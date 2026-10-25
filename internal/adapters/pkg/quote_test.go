package pkg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/r0madeus/veto/internal/core"
)

func TestRunCommand_QuotesArguments(t *testing.T) {
	mock := core.NewMockTransport()
	ctx := &core.SystemContext{Transport: mock, Context: context.Background()}

	_, _ = runCommand(ctx, "apt-get", "install", "-y", "curl; touch /tmp/pwned")
	_ = isInstalled(ctx, "dpkg", "-s", "$(id)")

	assert.Equal(t, "apt-get install -y 'curl; touch /tmp/pwned'", mock.Calls[0])
	assert.Equal(t, "dpkg -s '$(id)'", mock.Calls[1])
}
