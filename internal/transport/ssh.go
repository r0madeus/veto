package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/utils"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyCallback builds the host key verification callback for a host.
// Verification against known_hosts is the default; skipping it requires
// an explicit InsecureIgnoreHostKey opt-in.
func hostKeyCallback(host HostConfig) (ssh.HostKeyCallback, error) {
	if host.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	path := host.KnownHostsPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot locate known_hosts: %w", err)
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	}

	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("cannot load known_hosts (%s): %w; add the host with 'ssh-keyscan -p %d %s >> %s' or set insecure_host_key: true for this host", path, err, host.Port, host.Address, path)
	}
	return cb, nil
}

// sudoWrap runs the whole command line (including pipes and && chains) under
// sudo, reading the password from stdin without a prompt.
func sudoWrap(cmd string) string {
	return "sudo -S -p '' sh -c " + utils.ShellQuote(cmd)
}

type HostConfig struct {
	Name           string
	Address        string
	User           string
	Port           int
	SSHKeyPath     string
	BecomeMethod   string
	BecomePassword string // sudo password only; never used for SSH login
	SSHPassword    string // SSH login password (used when no key_path is set)

	// KnownHostsPath is the known_hosts file used to verify the server's host key.
	// Defaults to ~/.ssh/known_hosts.
	KnownHostsPath string
	// InsecureIgnoreHostKey disables host key verification. Opt-in only.
	InsecureIgnoreHostKey bool
}

type SSHTransport struct {
	client     *ssh.Client
	sftpClient *sftp.Client
	sftpFS     *SFTPFS
	config     HostConfig
	Logger     core.Logger
}

// authMethods builds the SSH login methods for a host. The sudo (become)
// password is never used for login; password auth needs SSHPassword.
func authMethods(host HostConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if host.SSHKeyPath != "" {
		key, err := os.ReadFile(host.SSHKeyPath)
		if err != nil {
			return nil, fmt.Errorf("ssh anahtarı okunamadı: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("ssh anahtarı parse edilemedi: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if host.SSHPassword != "" {
		methods = append(methods, ssh.Password(host.SSHPassword))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("no ssh credentials for host %s: set key_path or vars.ansible_ssh_pass", host.Name)
	}
	return methods, nil
}

func NewSSHTransport(ctx context.Context, host HostConfig) (*SSHTransport, error) {
	authMethods, err := authMethods(host)
	if err != nil {
		return nil, err
	}

	hostKeyCb, err := hostKeyCallback(host)
	if err != nil {
		return nil, fmt.Errorf("ssh host key setup failed (%s): %w", host.Name, err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCb,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host.Address, host.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		// DÜZELTME: host.HostName yerine host.Name kullanıldı
		return nil, fmt.Errorf("ssh bağlantı hatası (%s): %w", host.Name, err)
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("sftp başlatılamadı: %w", err)
	}

	return &SSHTransport{
		client:     client,
		sftpClient: sftpClient,
		sftpFS:     NewSFTPFS(sftpClient),
		config:     host,
	}, nil
}

func (t *SSHTransport) Close() error {
	if t.sftpClient != nil {
		t.sftpClient.Close()
	}
	if t.client != nil {
		return t.client.Close()
	}
	return nil
}

// Execute runs a command and returns its combined output.
func (t *SSHTransport) Execute(ctx context.Context, cmd string) (string, error) {
	if t.Logger != nil {
		t.Logger.Trace("Executing remote command", "host", t.config.Name, "command", cmd)
	}

	// Check if sudo is required
	if t.config.BecomeMethod == "sudo" {
		return t.runRemoteSecureCaptured(ctx, cmd, t.config.BecomePassword)
	}

	session, err := t.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// runRemoteSecureCaptured runs command with sudo and captures output
func (t *SSHTransport) runRemoteSecureCaptured(ctx context.Context, cmdStr string, sudoPassword string) (string, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	finalCmd := sudoWrap(cmdStr)

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf

	if err := session.Start(finalCmd); err != nil {
		return "", err
	}

	// Send password
	go func() {
		defer stdin.Close()
		if sudoPassword != "" {
			fmt.Fprintln(stdin, sudoPassword)
		}
	}()

	err = session.Wait()
	output := buf.String()

	return output, err
}

// RunRemoteSecure: Komutu çalıştırır, sudo gerekirse şifreyi pipe ile verir.
// Bu fonksiyon, parolanın process listesinde veya history'de görünmesini engeller.
func (t *SSHTransport) RunRemoteSecure(ctx context.Context, cmdStr string, sudoPassword string, stdinData string) error {
	session, err := t.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	// Eğer sudo kullanılıyorsa, komutu şifre isteyecek şekilde (promptsuz) ayarla
	finalCmd := cmdStr
	if t.config.BecomeMethod == "sudo" && sudoPassword != "" {
		// -S: Stdin'den şifre oku
		// -p '': Prompt gösterme
		finalCmd = sudoWrap(cmdStr)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	// Çıktıları yönlendir
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Start(finalCmd); err != nil {
		return err
	}

	// Stdin yönetimi (Şifre ve veri gönderimi)
	go func() {
		defer stdin.Close()
		// 1. Önce sudo şifresini gönder (Güvenli aktarım)
		if t.config.BecomeMethod == "sudo" && sudoPassword != "" {
			fmt.Fprintln(stdin, sudoPassword)
		}
		// 2. Sonra asıl datayı gönder (varsa)
		if stdinData != "" {
			io.WriteString(stdin, stdinData)
		}
	}()

	return session.Wait()
}

func (t *SSHTransport) CopyFile(ctx context.Context, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return err
	}

	remoteFile, err := t.sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return err
	}

	return remoteFile.Chmod(stat.Mode().Perm())
}

func (t *SSHTransport) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	remoteFile, err := t.sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	localFile, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	_, err = io.Copy(localFile, remoteFile)
	return err
}

func (t *SSHTransport) GetFileSystem() core.FileSystem {
	return t.sftpFS
}

func (t *SSHTransport) GetOS(ctx context.Context) (string, error) {
	osName, _, err := t.GetRemoteSystemInfo(ctx)
	return osName, err
}

func (t *SSHTransport) GetRemoteSystemInfo(ctx context.Context) (string, string, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	// OS ve Arch bilgisini öğren
	out, err := session.Output("uname -s -m")
	if err != nil {
		return "", "", err
	}
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return "", "", fmt.Errorf("bilinmeyen sistem çıktısı: %s", string(out))
	}

	osName := strings.ToLower(parts[0]) // Linux -> linux
	arch := strings.ToLower(parts[1])   // x86_64 -> amd64

	// Mimari isimlerini Go standartlarına çevir
	if arch == "x86_64" {
		arch = "amd64"
	} else if arch == "aarch64" {
		arch = "arm64"
	}

	return osName, arch, nil
}
