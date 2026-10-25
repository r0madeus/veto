package main

import (
	"os"

	"github.com/r0madeus/veto/cmd"
)

func main() {
	// cobra has already printed the error (unknown command, bad flag, ...).
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
