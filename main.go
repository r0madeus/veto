package main

import (
	"os"

	"github.com/melih-ucgun/veto/cmd"
)

func main() {
	// cobra has already printed the error (unknown command, bad flag, ...).
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
