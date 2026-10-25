package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTarGz(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(content))}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return &buf
}

func TestExtractTarGz_Valid(t *testing.T) {
	dest := t.TempDir()
	err := ExtractTarGz(buildTarGz(t, map[string]string{"sub/a.txt": "hello"}), dest)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dest, "sub", "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestExtractTarGz_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "dest")
	require.NoError(t, os.MkdirAll(dest, 0755))

	err := ExtractTarGz(buildTarGz(t, map[string]string{"../evil.txt": "pwned"}), dest)
	assert.Error(t, err)
	_, statErr := os.Stat(filepath.Join(root, "evil.txt"))
	assert.True(t, os.IsNotExist(statErr), "file must not be written outside destDir")
}
