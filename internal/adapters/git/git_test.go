package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/melih-ucgun/veto/internal/core"
)

func TestGitAdapter_Check_Clone(t *testing.T) {
	// Setup Context with Mock Transport
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "my-repo")

	// Params
	params := map[string]interface{}{
		"repo": "https://github.com/example/repo",
		"dest": repoPath,
	}

	adapter := NewGitAdapter("test-git", params)

	// Since folder does not exist, Check should return True (Clone needed)
	needsAction, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !needsAction {
		t.Fatal("Expected needsAction=true for missing repo")
	}
}

func TestGitAdapter_Check_UpToDate(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "existing-repo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0755); err != nil { // Simulate git repo
		t.Fatal(err)
	}

	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	// Mock Git Commands
	mockTransport.OnExecute("git -C "+repoPath+" remote get-url origin", "https://github.com/example/repo", nil)

	params := map[string]interface{}{
		"repo": "https://github.com/example/repo",
		"dest": repoPath,
	}

	adapter := NewGitAdapter("test-git-exists", params)

	needsAction, err := adapter.Check(ctx)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if needsAction {
		t.Fatal("Expected needsAction=false for existing correct repo")
	}
}

func TestGitAdapter_Apply_Clone(t *testing.T) {
	mockTransport := core.NewMockTransport()
	ctx := &core.SystemContext{
		FS:        &core.RealFS{},
		Transport: mockTransport,
		Logger:    core.NewDefaultLogger(os.Stderr, core.LevelDebug),
	}

	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "new-repo")

	// Mock Clone
	expectedCmd := "git clone -b main -- https://github.com/example/repo " + repoPath
	mockTransport.OnExecute(expectedCmd, "", nil)

	params := map[string]interface{}{
		"repo":   "https://github.com/example/repo",
		"dest":   repoPath,
		"branch": "main",
	}

	adapter := NewGitAdapter("apply-clone", params)

	result, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected Changed=true")
	}

	if !mockTransport.AssertCalled("clone") {
		t.Fatal("Git clone was not called")
	}
}

func TestGitHelpers_QuoteArguments(t *testing.T) {
	mock := core.NewMockTransport()
	ctx := &core.SystemContext{Transport: mock, Context: context.Background()}

	_, _ = getRemoteURL(ctx, "/tmp/my repo", "origin; id")
	_, _ = getRefSHA(ctx, "/tmp/repo", "$(touch /tmp/pwned)")
	_ = checkout(ctx, "/tmp/repo", "main && id")

	assert.Equal(t, "git -C '/tmp/my repo' remote get-url 'origin; id'", mock.Calls[0])
	assert.Equal(t, "git -C /tmp/repo rev-parse '$(touch /tmp/pwned)'", mock.Calls[1])
	assert.Equal(t, "git -C /tmp/repo checkout 'main && id'", mock.Calls[2])
}

func TestGitAdapter_Validate_RejectsOptionLikeValues(t *testing.T) {
	ctx := &core.SystemContext{}
	for _, params := range []map[string]interface{}{
		{"repo": "--upload-pack=evil", "dest": "/tmp/x"},
		{"repo": "https://example.com/r.git", "dest": "/tmp/x", "branch": "-b"},
		{"repo": "https://example.com/r.git", "dest": "/tmp/x", "remote": "--force"},
	} {
		if err := NewGitAdapter("g", params).Validate(ctx); err == nil {
			t.Errorf("expected validation error for %v", params)
		}
	}
	ok := NewGitAdapter("g", map[string]interface{}{"repo": "https://example.com/r.git", "dest": "/tmp/x"})
	if err := ok.Validate(ctx); err != nil {
		t.Errorf("valid params rejected: %v", err)
	}
}
