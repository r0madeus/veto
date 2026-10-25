package discovery

import (
	"bufio"
	"bytes"
	"github.com/melih-ucgun/veto/internal/utils"
	"strings"

	"github.com/melih-ucgun/veto/internal/core"
)

func isCommandAvailable(ctx *core.SystemContext, name string) bool {
	_, err := ctx.Transport.Execute(ctx.Context, "which "+utils.ShellQuote(name))
	return err == nil
}

func parseLines(data []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func unique(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
