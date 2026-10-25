package state

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/r0madeus/veto/internal/consts"
)

// BackupManager handles creating copies of files before modification
type BackupManager struct {
	BaseDir string
}

func NewBackupManager(baseDir string) *BackupManager {
	if baseDir == "" {
		// Default to .veto/backups relative to user home is risky if not configured right,
		// but Manager usually passes the base state path.
		// Let's assume absolute path passed or handle standard location.
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, consts.GetVetoDir(), consts.BackupDirName)
	}
	return &BackupManager{BaseDir: baseDir}
}

// CreateBackup copies the source file to the backup directory associated with the txID.
// Returns the absolute path to the backup file.
func (bm *BackupManager) CreateBackup(txID, sourcePath string) (string, error) {
	// 1. Verify source exists
	info, err := os.Stat(sourcePath)
	if os.IsNotExist(err) {
		return "", nil // Nothing to backup
	}
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("backup source '%s' is a directory, not supported", sourcePath)
	}

	// 2. Prepare backup path: baseDir / txID / <hash_of_path>
	// Hashing the path ensures unique filenames without directory depth issues
	pathHash := fmt.Sprintf("%x", sha256.Sum256([]byte(sourcePath)))
	backupDir := filepath.Join(bm.BaseDir, txID)
	backupPath := filepath.Join(backupDir, pathHash)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}

	// 3. Copy file
	src, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	return backupPath, nil
}

// RestoreBackup copies the backup file back to the target destination
func (bm *BackupManager) RestoreBackup(backupPath, targetPath string) error {
	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found at %s: %w", backupPath, err)
	}

	// Ensure target dir exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}

	// Copy back
	src, err := os.Open(backupPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
