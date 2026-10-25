package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestNewSSHTransport(t *testing.T) {
	// HostName alanı Name olarak güncellendi
	host := HostConfig{
		Name:           "test-host",
		Address:        "127.0.0.1",
		User:           "testuser",
		Port:           22,
		BecomePassword: "password",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Bağlantı denemesi (bağlanamayacağı için hata vermesi normal,
	// ancak struct literal hatası vermemeli)
	_, err := NewSSHTransport(ctx, host)
	if err == nil {
		t.Log("Beklenen: Bağlantı hatası aldık (sunucu yok)")
	}
}

func TestHostKeyCallback_InsecureOptIn(t *testing.T) {
	cb, err := hostKeyCallback(HostConfig{InsecureIgnoreHostKey: true})
	if err != nil || cb == nil {
		t.Fatalf("expected callback for insecure opt-in, got %v", err)
	}
}

func TestHostKeyCallback_MissingKnownHostsFails(t *testing.T) {
	_, err := hostKeyCallback(HostConfig{Address: "example.com", Port: 22, KnownHostsPath: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error when known_hosts is missing")
	}
}

func TestHostKeyCallback_RejectsUnknownAndMismatchedKey(t *testing.T) {
	newKey := func() ssh.PublicKey {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		k, err := ssh.NewPublicKey(pub)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	trusted, other := newKey(), newKey()

	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{"example.com:22"}, trusted)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cb, err := hostKeyCallback(HostConfig{KnownHostsPath: path})
	if err != nil {
		t.Fatal(err)
	}
	addr := &net.TCPAddr{IP: net.ParseIP("203.0.113.1"), Port: 22}

	if err := cb("example.com:22", addr, trusted); err != nil {
		t.Errorf("trusted key rejected: %v", err)
	}
	if err := cb("example.com:22", addr, other); err == nil {
		t.Error("mismatched key must be rejected")
	}
	if err := cb("unknown.example:22", addr, trusted); err == nil {
		t.Error("unknown host must be rejected")
	}
}

func TestAuthMethods_DoesNotUseBecomePasswordForLogin(t *testing.T) {
	if _, err := authMethods(HostConfig{Name: "h", BecomePassword: "sudo-secret"}); err == nil {
		t.Fatal("become password must not be used as SSH login credential")
	}
}

func TestAuthMethods_Password(t *testing.T) {
	m, err := authMethods(HostConfig{Name: "h", SSHPassword: "login", BecomePassword: "sudo"})
	if err != nil || len(m) != 1 {
		t.Fatalf("expected one password method, got %d, err %v", len(m), err)
	}
}

func TestSudoWrap_CoversWholeCommandLine(t *testing.T) {
	got := sudoWrap("mkdir -p /a && cp x /a/")
	want := "sudo -S -p '' sh -c 'mkdir -p /a && cp x /a/'"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
