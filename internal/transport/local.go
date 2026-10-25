package transport

import (
	"context"
	"runtime"

	"github.com/r0madeus/veto/internal/core"
)

// LocalTransport implements Transport for the local machine
type LocalTransport struct {
	Logger core.Logger
}

func NewLocalTransport() *LocalTransport {
	return &LocalTransport{}
}

func (t *LocalTransport) Close() error {
	return nil
}

func (t *LocalTransport) Execute(ctx context.Context, cmd string) (string, error) {
	if t.Logger != nil {
		t.Logger.Trace("Executing local command", "command", cmd)
	}
	// For local execution, we use the global CommandRunner.
	// We wrap the command string in a shell to ensure compatibility with remote execution.
	return core.RunCommand("sh", "-c", cmd)
}

func (t *LocalTransport) CopyFile(ctx context.Context, localPath, remotePath string) error {
	// For local transport, localPath and remotePath are on the same machine.
	fs := &core.RealFS{}
	return core.CopyFile(fs, localPath, remotePath, 0644)
}

func (t *LocalTransport) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	// Local download is just a copy.
	return t.CopyFile(ctx, remotePath, localPath)
}

func (t *LocalTransport) GetFileSystem() core.FileSystem {
	return &core.RealFS{}
}

func (t *LocalTransport) GetOS(ctx context.Context) (string, error) {
	return runtime.GOOS, nil
}
