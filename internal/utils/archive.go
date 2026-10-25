package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// safeJoin joins name onto destDir and rejects results that escape destDir (Zip Slip).
func safeJoin(destDir, name string) (string, error) {
	target := filepath.Join(destDir, name)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("illegal file path in archive: %s", name)
	}
	return target, nil
}

// ExtractTarGz extracts a tar.gz stream to the destination directory.
func ExtractTarGz(gzipStream io.Reader, destDir string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return fmt.Errorf("ExtractTarGz: NewReader failed: %w", err)
	}
	defer uncompressedStream.Close()

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ExtractTarGz: Next() failed: %w", err)
		}

		targetPath, err := safeJoin(destDir, header.Name)
		if err != nil {
			return fmt.Errorf("ExtractTarGz: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("ExtractTarGz: Mkdir() failed: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("ExtractTarGz: Mkdir() for file failed: %w", err)
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("ExtractTarGz: Create() failed: %w", err)
			}

			// Copy content
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("ExtractTarGz: Copy() failed: %w", err)
			}
			outFile.Close()
		default:
			// Ignore other types (symlinks, etc.) for now to keep it simple and safe
			// unless explicitly needed.
		}
	}

	return nil
}
