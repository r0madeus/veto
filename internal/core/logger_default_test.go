package core

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultLogger_SetLevelEnablesDebug(t *testing.T) {
	var buf bytes.Buffer
	l := NewDefaultLogger(&buf, LevelInfo)

	l.Debug("hidden-at-info")
	if strings.Contains(buf.String(), "hidden-at-info") {
		t.Fatal("debug message must not appear at info level")
	}

	l.SetLevel(LevelDebug) // what `-vv` does
	l.Debug("visible-at-debug")
	out := buf.String()
	if strings.Count(out, "visible-at-debug") < 2 { // pterm line + slog line
		t.Fatalf("debug message missing after SetLevel(Debug): %q", out)
	}
}
