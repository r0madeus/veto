package utils

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellQuote(t *testing.T) {
	assert.Equal(t, "vim", ShellQuote("vim"))
	assert.Equal(t, "/etc/foo-1.conf", ShellQuote("/etc/foo-1.conf"))
	assert.Equal(t, "''", ShellQuote(""))
	assert.Equal(t, "'my dir'", ShellQuote("my dir"))
	assert.Equal(t, `'it'\''s'`, ShellQuote("it's"))
	assert.Equal(t, "'a; rm -rf /'", ShellQuote("a; rm -rf /"))
}

func TestShellJoin(t *testing.T) {
	assert.Equal(t, "git clone 'a b' dest", ShellJoin("git", "clone", "a b", "dest"))
}

// Round-trip through a real shell: every argument must arrive as exactly one word.
func TestShellQuote_RoundTrip(t *testing.T) {
	inputs := []string{"plain", "a b", "it's", `$(touch /tmp/pwned)`, "a;b", "`id`", "*", "x\ny", `back\slash`, "%{NAME}\\n", "--opt=va lue"}
	for _, in := range inputs {
		out, err := exec.Command("sh", "-c", "printf %s "+ShellQuote(in)).Output()
		require.NoError(t, err)
		assert.Equal(t, in, string(out))
	}
}
