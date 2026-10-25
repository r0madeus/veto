package transport

import (
	"io"
	"os"

	"github.com/pkg/sftp"
	"github.com/r0madeus/veto/internal/core"
)

// SFTPFS implements core.FileSystem over an SFTP connection
type SFTPFS struct {
	client *sftp.Client
}

func NewSFTPFS(client *sftp.Client) *SFTPFS {
	return &SFTPFS{client: client}
}

func (fs *SFTPFS) Create(name string) (core.File, error) {
	f, err := fs.client.Create(name)
	if err != nil {
		return nil, err
	}
	return &SFTPFile{File: f}, nil
}

func (fs *SFTPFS) Open(name string) (core.File, error) {
	f, err := fs.client.Open(name)
	if err != nil {
		return nil, err
	}
	return &SFTPFile{File: f}, nil
}

func (fs *SFTPFS) Stat(name string) (os.FileInfo, error) {
	return fs.client.Stat(name)
}

func (fs *SFTPFS) Lstat(name string) (os.FileInfo, error) {
	return fs.client.Lstat(name)
}

func (fs *SFTPFS) MkdirAll(path string, perm os.FileMode) error {
	return fs.client.MkdirAll(path)
}

func (fs *SFTPFS) Remove(name string) error {
	return fs.client.Remove(name)
}

func (fs *SFTPFS) RemoveAll(path string) error {
	// SFTP doesn't have a direct RemoveAll. We need to implement it recursively
	// or use a shell command. For simplicity and reliability in agentless mode,
	// we use the client's Walker or direct shell if available via transport.
	// However, SFTPFS only has the sftp client.

	// Implementation note: a simple recursive removal via SFTP walker
	walker := fs.client.Walk(path)
	var paths []string
	for walker.Step() {
		if walker.Err() != nil {
			continue
		}
		paths = append(paths, walker.Path())
	}

	// Remove in reverse order (files/subdirs first)
	for i := len(paths) - 1; i >= 0; i-- {
		_ = fs.client.Remove(paths[i])
	}
	return nil
}

func (fs *SFTPFS) Rename(oldpath, newpath string) error {
	return fs.client.Rename(oldpath, newpath)
}

func (fs *SFTPFS) Chmod(name string, mode os.FileMode) error {
	return fs.client.Chmod(name, mode)
}

func (fs *SFTPFS) Chown(name string, uid, gid int) error {
	return fs.client.Chown(name, uid, gid)
}

func (fs *SFTPFS) ReadDir(dirname string) ([]os.DirEntry, error) {
	infos, err := fs.client.ReadDir(dirname)
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = &sftpDirEntry{info}
	}
	return entries, nil
}

func (fs *SFTPFS) ReadFile(filename string) ([]byte, error) {
	f, err := fs.client.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (fs *SFTPFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	f, err := fs.client.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return fs.client.Chmod(filename, perm)
}

func (fs *SFTPFS) Symlink(oldname, newname string) error {
	return fs.client.Symlink(oldname, newname)
}

func (fs *SFTPFS) Readlink(name string) (string, error) {
	return fs.client.ReadLink(name)
}

// SFTPFile wraps *sftp.File to satisfy core.File
type SFTPFile struct {
	*sftp.File
}

func (f *SFTPFile) Stat() (os.FileInfo, error) {
	return f.File.Stat()
}

// sftpDirEntry wraps os.FileInfo to satisfy os.DirEntry
type sftpDirEntry struct {
	os.FileInfo
}

func (d *sftpDirEntry) Type() os.FileMode {
	return d.Mode().Type()
}

func (d *sftpDirEntry) Info() (os.FileInfo, error) {
	return d.FileInfo, nil
}
