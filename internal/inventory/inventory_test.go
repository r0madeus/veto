package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/r0madeus/veto/internal/crypto"
)

func TestLoadInventory_Defaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inv.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`hosts:
  - name: local
    address: 127.0.0.1
    user: me
  - name: web
    address: 192.0.2.10
    user: deploy
    port: 2222
`), 0600))

	inv, err := LoadInventory(path)
	require.NoError(t, err)
	require.Len(t, inv.Hosts, 2)
	assert.Equal(t, "local", inv.Hosts[0].Connection)
	assert.Equal(t, 22, inv.Hosts[0].Port)
	assert.Equal(t, "ssh", inv.Hosts[1].Connection)
	assert.Equal(t, 2222, inv.Hosts[1].Port)
}

func TestLoadInventory_MissingFile(t *testing.T) {
	_, err := LoadInventory(filepath.Join(t.TempDir(), "nope.yaml"))
	assert.Error(t, err)
}

func TestDecryptVars(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	enc, err := crypto.Encrypt("s3cret", key)
	require.NoError(t, err)

	inv := &Inventory{Hosts: []Host{{Name: "h", Vars: map[string]string{"ansible_ssh_pass": enc, "plain": "x"}}}}
	assert.True(t, inv.HasEncryptedVars())
	require.NoError(t, inv.DecryptVars(key))
	assert.Equal(t, "s3cret", inv.Hosts[0].Vars["ansible_ssh_pass"])
	assert.Equal(t, "x", inv.Hosts[0].Vars["plain"])
	assert.False(t, inv.HasEncryptedVars())
}

func TestDecryptVars_WrongKeyFails(t *testing.T) {
	key, _ := crypto.GenerateKey()
	other, _ := crypto.GenerateKey()
	enc, err := crypto.Encrypt("s3cret", key)
	require.NoError(t, err)

	inv := &Inventory{Hosts: []Host{{Name: "h", Vars: map[string]string{"ansible_ssh_pass": enc}}}}
	assert.Error(t, inv.DecryptVars(other))
	assert.Equal(t, enc, inv.Hosts[0].Vars["ansible_ssh_pass"], "ciphertext must stay untouched on failure")
}
