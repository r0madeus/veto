package fleet

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/inventory"
	"github.com/melih-ucgun/veto/internal/transport"
	"github.com/pterm/pterm"
)

// Executor handles ad-hoc command execution across a fleet
type Executor struct {
	hosts       []inventory.Host
	concurrency int
	useSudo     bool
}

// NewExecutor creates a new fleet executor
func NewExecutor(hosts []inventory.Host, concurrency int, useSudo bool) *Executor {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Executor{
		hosts:       hosts,
		concurrency: concurrency,
		useSudo:     useSudo,
	}
}

// Run executes the given shell command on all hosts
func (e *Executor) Run(command string) error {
	pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgMagenta)).Printf("Fleet Exec: '%s'", command)
	pterm.Println()

	var wg sync.WaitGroup
	sem := make(chan struct{}, e.concurrency)
	errChan := make(chan error, len(e.hosts))

	for _, host := range e.hosts {
		wg.Add(1)
		go func(h inventory.Host) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := e.runOnHost(h, command); err != nil {
				errChan <- fmt.Errorf("[%s] failed: %w", h.Name, err)
			}
		}(host)
	}

	wg.Wait()
	close(errChan)

	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	pterm.Println()
	if len(errs) > 0 {
		return fmt.Errorf("execution failed on %d/%d hosts", len(errs), len(e.hosts))
	}

	pterm.Success.Printf("Command executed successfully on %d hosts.\n", len(e.hosts))
	return nil
}

func (e *Executor) runOnHost(h inventory.Host, cmd string) error {
	// 1. Prepare Transport Config
	cfgHost := transport.HostConfig{
		Name:       h.Name,
		Address:    h.Address,
		User:       h.User,
		Port:       h.Port,
		SSHKeyPath: h.KeyPath,

		KnownHostsPath:        h.KnownHosts,
		InsecureIgnoreHostKey: h.InsecureHostKey,
	}

	if e.useSudo {
		cfgHost.BecomeMethod = "sudo"
		cfgHost.BecomePassword = h.Vars["ansible_become_password"] // Ensure this is populated
	}

	// 2. Initialize Context & Transport
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30s timeout per command? Configurable?
	defer cancel()

	var tr core.Transport
	var err error

	if h.Connection == "ssh" {
		tr, err = transport.NewSSHTransport(ctx, cfgHost)
	} else {
		tr = transport.NewLocalTransport()
	}

	if err != nil {
		pterm.Error.Printf("[%s] Connection failed: %v\n", h.Name, err)
		return err
	}
	defer tr.Close()

	// 3. Execute Command
	// Transport.Execute returns stdout/stderr combined
	output, err := tr.Execute(ctx, cmd)
	if err != nil {
		pterm.Error.Printf("[%s] Failed: %v\n", h.Name, err)
		if output != "" {
			// Print output even on failure if any
			lines := strings.Split(strings.TrimSpace(output), "\n")
			for _, line := range lines {
				pterm.Printf("[%s] %s\n", h.Name, line)
			}
		}
		return err
	}

	// Print successful output
	if output != "" {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			pterm.Printf("[%s] %s\n", h.Name, line)
		}
	}

	pterm.Success.Printf("[%s] Done\n", h.Name)
	return nil
}
