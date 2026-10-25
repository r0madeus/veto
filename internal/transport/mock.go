package transport

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/r0madeus/veto/internal/core"
)

// MockTransport simulates a transport layer for testing.
type MockTransport struct {
	mu          sync.Mutex
	Responses   map[string]string // Command -> Output
	Errors      map[string]error  // Command -> Error
	FileContent map[string]string // FilePath -> Content
	CopiedFiles map[string]string // Src -> Dst (Record of copies)
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		Responses:   make(map[string]string),
		Errors:      make(map[string]error),
		FileContent: make(map[string]string),
		CopiedFiles: make(map[string]string),
	}
}

// AddResponse registers a canned response for a command.
func (m *MockTransport) AddResponse(cmd, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[cmd] = output
}

// AddError registers a canned error for a command.
func (m *MockTransport) AddError(cmd string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors[cmd] = err
}

func (m *MockTransport) Execute(ctx context.Context, cmd string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for error first
	if err, ok := m.Errors[cmd]; ok {
		return "", err
	}

	// Check for response
	if output, ok := m.Responses[cmd]; ok {
		return output, nil
	}

	return "", fmt.Errorf("mock: command not mocked: %s", cmd)
}

func (m *MockTransport) CopyFile(ctx context.Context, localPath, remotePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CopiedFiles[localPath] = remotePath
	return nil
}

func (m *MockTransport) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	// For testing, we might want to simulate download.
	// But mostly we just assume it worked.
	return nil
}

func (m *MockTransport) GetOS(ctx context.Context) (string, error) {
	return "linux", nil
}

func (m *MockTransport) GetFileSystem() core.FileSystem {
	return &MockFileSystem{Content: m.FileContent}
}

func (m *MockTransport) Close() error {
	return nil
}

// MockFileSystem implements core.FileSystem
type MockFileSystem struct {
	Content map[string]string
}

func (m *MockFileSystem) Open(name string) (core.File, error) {
	if content, ok := m.Content[name]; ok {
		return NewMockFile(name, []byte(content)), nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Create(name string) (core.File, error) {
	return NewMockFile(name, []byte{}), nil
}

func (m *MockFileSystem) Stat(name string) (fs.FileInfo, error) {
	if content, ok := m.Content[name]; ok {
		return &mockFileInfo{name: name, size: int64(len(content))}, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) ReadFile(name string) ([]byte, error) {
	if content, ok := m.Content[name]; ok {
		return []byte(content), nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Readlink(name string) (string, error)         { return "", nil }
func (m *MockFileSystem) Symlink(oldname, newname string) error        { return nil }
func (m *MockFileSystem) Remove(name string) error                     { return nil }
func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error { return nil }
func (m *MockFileSystem) Chmod(path string, perm os.FileMode) error    { return nil }
func (m *MockFileSystem) Chown(path string, uid, gid int) error        { return nil }

// Other methods needed for FileSystem interface?
// core.FileSystem interface:
// Stat, Lstat, ReadFile, WriteFile, MkdirAll, Remove, RemoveAll, Readlink, Symlink, Chmod, Open, Create, ReadDir
func (m *MockFileSystem) Lstat(name string) (fs.FileInfo, error) { return m.Stat(name) }
func (m *MockFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	m.Content[name] = string(data)
	return nil
}
func (m *MockFileSystem) RemoveAll(path string) error                { return nil }
func (m *MockFileSystem) ReadDir(name string) ([]fs.DirEntry, error) { return nil, nil }

// MockFile implements core.File
type MockFile struct {
	NameStr string
	Buffer  *bytes.Buffer
	Reader  *bytes.Reader
}

func NewMockFile(name string, data []byte) *MockFile {
	return &MockFile{
		NameStr: name,
		Buffer:  bytes.NewBuffer(data),
		Reader:  bytes.NewReader(data),
	}
}

func (f *MockFile) Read(p []byte) (n int, err error) {
	return f.Reader.Read(p)
}

func (f *MockFile) ReadAt(p []byte, off int64) (n int, err error) {
	return f.Reader.ReadAt(p, off)
}

func (f *MockFile) Write(p []byte) (n int, err error) {
	return f.Buffer.Write(p)
}

func (f *MockFile) Close() error {
	return nil
}

func (f *MockFile) Stat() (fs.FileInfo, error) {
	return &mockFileInfo{name: f.NameStr, size: int64(f.Reader.Len())}, nil
}

type mockFileInfo struct {
	name string
	size int64
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() interface{}   { return nil }
