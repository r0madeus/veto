package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/r0madeus/veto/internal/core"
)

// MockBackupManager implements core.BackupManager interface
type MockBackupManager struct {
	CreateBackupCalled    bool
	RestoreBackupCalled   bool
	LastCreateBackupSrc   string
	LastRestoreBackupPath string
}

func (m *MockBackupManager) CreateBackup(txID, srcPath string) (string, error) {
	m.CreateBackupCalled = true
	m.LastCreateBackupSrc = srcPath
	return "/backup/path", nil
}

func (m *MockBackupManager) RestoreBackup(backupPath, destPath string) error {
	m.RestoreBackupCalled = true
	m.LastRestoreBackupPath = backupPath
	return nil
}

func TestFileAdapter_Apply_Backup(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "veto-file-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(targetFile, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	mockBM := &MockBackupManager{}
	ctx := core.NewSystemContext(false, nil)
	ctx.BackupManager = mockBM
	ctx.TxID = "tx1"

	// Init Adapter
	adapter := NewFileAdapter(targetFile, map[string]interface{}{
		"content": "new content",
	})

	// Run Apply
	res, err := adapter.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Verify Backup was requested
	if !mockBM.CreateBackupCalled {
		t.Error("BackupManager.CreateBackup was not called")
	}
	if mockBM.LastCreateBackupSrc != targetFile {
		t.Errorf("Backup source mismatch. Got %s, want %s", mockBM.LastCreateBackupSrc, targetFile)
	}

	if !res.Changed {
		t.Error("Expected status changed")
	}
}

func TestFileAdapter_RevertAction(t *testing.T) {
	mockBM := &MockBackupManager{}
	ctx := core.NewSystemContext(false, nil)
	ctx.BackupManager = mockBM

	// Init Adapter
	res := NewFileAdapter("/tmp/test.txt", nil)
	// Cast to concrete type to access struct fields for testing
	adapter, ok := res.(*FileAdapter)
	if !ok {
		t.Fatal("Failed to cast resource to *FileAdapter")
	}
	adapter.BackupPath = "/backup/path/hash"

	// Test Reverting a Modification (which uses backup)
	err := adapter.RevertAction("modified", ctx)
	if err != nil {
		t.Fatalf("RevertAction failed: %v", err)
	}

	if !mockBM.RestoreBackupCalled {
		t.Error("RestoreBackup was not called for 'modified' action")
	}
	if mockBM.LastRestoreBackupPath != "/backup/path/hash" {
		t.Errorf("Restore path mismatch")
	}

	// Test Reverting Creation (should remove file)
	// For this test we need a real file to delete to check os.Remove logic,
	// or mock os.Remove if we want pure unit test.
	// Since adapter uses os.Remove directly, integration style is easier.
	tmpFile := "/tmp/veto_test_delete_me.txt"
	if err := os.WriteFile(tmpFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	res2 := NewFileAdapter(tmpFile, nil)
	adapter2, ok := res2.(*FileAdapter)
	if !ok {
		t.Fatal("Failed to cast resource to *FileAdapter")
	}

	err = adapter2.RevertAction("created", ctx)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("File was not deleted on revert creation")
	}
}
