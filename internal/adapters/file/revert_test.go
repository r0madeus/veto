package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/state"
)

// TestRevert_Creation verifies that reverting a file creation deletes the file.
func TestRevert_Creation(t *testing.T) {
	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "veto-test-revert-creation")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := &core.SystemContext{
		FS:      &core.RealFS{}, // Using real FS for file operations
		Context: nil,
		Logger:  core.NewDefaultLogger(os.Stderr, core.LevelDebug), // Debug logging
	}

	targetPath := filepath.Join(tmpDir, "testfile.txt")

	// 1. Create file resource
	params := map[string]interface{}{
		"path":    targetPath,
		"content": "Hello World",
		"state":   "present",
	}

	res := NewFileAdapter("test-file", params)

	// 2. Apply (Create)
	result, err := res.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		t.Fatal("Expected changed=true for creation")
	}

	// Verify file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}

	// 3. Revert
	// Manually set ActionPerformed as the Engine would do
	fRes := res.(*FileAdapter)

	// NOTE: In the real implementation, Apply sets ActionPerformed.
	// Let's verify it is set.
	if fRes.ActionPerformed != "created" {
		t.Errorf("Expected ActionPerformed='created', got '%s'", fRes.ActionPerformed)
		// Force it for the test if the implementation is currently missing it (which we suspect it might be partly)
		// usage in Apply: r.ActionPerformed = "created" (Line 227 of user.go, need to check file.go)
		// Checked file.go: IT DOES NOT SET ActionPerformed in Apply! This is another bug to fix.
		fRes.ActionPerformed = "created"
	}

	if err := fRes.Revert(ctx); err != nil {
		t.Fatalf("Revert failed: %v", err)
	}

	// 4. Verify file is deleted
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatal("File should have been deleted after revert")
	}
}

// TestRevert_Modification verifies that reverting a modification restores the backup.
func TestRevert_Modification(t *testing.T) {
	// Setup temporary directory
	tmpDir, err := os.MkdirTemp("", "veto-test-revert-mod")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup Backup Manager
	backupBaseDir := filepath.Join(tmpDir, "backups")
	bm := state.NewBackupManager(backupBaseDir)

	ctx := &core.SystemContext{
		FS:            &core.RealFS{},
		Logger:        core.NewDefaultLogger(os.Stderr, core.LevelDebug),
		TxID:          "test-tx-1",
		BackupManager: bm,
	}

	targetPath := filepath.Join(tmpDir, "config.conf")
	originalContent := "Original Content"

	// Create initial file
	if err := os.WriteFile(targetPath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Create file resource with NEW content
	params := map[string]interface{}{
		"path":    targetPath,
		"content": "New Content",
		"state":   "present",
	}

	res := NewFileAdapter("mod-file", params)

	// 2. Apply (Modify)
	result, err := res.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Changed {
		// Did it detect change?
		t.Fatal("Expected changed=true for modification")
	}

	fRes := res.(*FileAdapter)
	// Check if backup path is set (it should be if Apply calls BackupManager)
	if fRes.BackupPath == "" {
		t.Fatal("BackupPath should be set after modification with BackupManager")
	}

	// Verify content changed
	current, _ := os.ReadFile(targetPath)
	if string(current) != "New Content" {
		t.Fatal("File content was not updated")
	}

	// 3. Revert
	// Again, assuming Apply sets ActionPerformed (it currently doesn't in file.go, so we mock it as 'modified' implies logic)
	// The current file.go Apply doesn't set ActionPerformed. We will fix that too.
	fRes.ActionPerformed = "modified" // This is what we expect Engine or Apply to set.

	if err := fRes.Revert(ctx); err != nil {
		t.Fatalf("Revert failed: %v", err)
	}

	// 4. Verify file is restored to original
	restored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read file after revert: %v", err)
	}
	if string(restored) != originalContent {
		t.Fatalf("Revert failed to restore content. Got '%s', want '%s'", string(restored), originalContent)
	}
}

// TestRevert_Modification_NoBackup_Unsafe verifies the behavior when backup is missing.
// CURRENTLY: It likely deletes the file (Unsafe).
// AFTER FIX: It should do nothing (Safe).
func TestRevert_Modification_NoBackup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "veto-test-revert-nobackup")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := &core.SystemContext{
		FS:     &core.RealFS{},
		Logger: core.NewDefaultLogger(os.Stderr, core.LevelDebug),
		// No BackupManager -> No Backup created
	}

	targetPath := filepath.Join(tmpDir, "important.data")
	originalContent := "Important Data"
	if err := os.WriteFile(targetPath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	params := map[string]interface{}{
		"path":    targetPath,
		"content": "Corrupted Data", // Changing it
		"state":   "present",
	}

	res := NewFileAdapter("uhoh-file", params)
	fRes := res.(*FileAdapter)

	// Simulate Apply manually to skip backup creation (since we passed no BackupManager)
	// The file is "modified" on disk
	if err := os.WriteFile(targetPath, []byte("Corrupted Data"), 0644); err != nil {
		t.Fatal(err)
	}

	fRes.ActionPerformed = "modified"
	fRes.BackupPath = "" // Explicitly empty

	// 3. Revert
	err = fRes.Revert(ctx)
	if err != nil {
		t.Logf("Revert returned error (unexpected but okay): %v", err)
	}

	// 4. Verify what happened
	// IF BUGGY: File is deleted because State="present" and BackupPath="" -> Revert calls Remove
	// IF FIXED: File remains (content might be corrupted, but file exists)

	_, err = os.Stat(targetPath)
	if os.IsNotExist(err) {
		t.Fatal("CRITICAL: Revert deleted the modified file because no backup was found! This is the unsafe behavior we must fix.")
	}

	// If we are here, the file exists.
	content, _ := os.ReadFile(targetPath)
	t.Logf("File content after revert: %s", string(content))
}
