package snapshot

import (
	"github.com/pterm/pterm"
	"github.com/r0madeus/veto/internal/core"
)

// Manager handles the selection and execution of snapshot providers
type Manager struct {
	provider Provider
}

// NewManager detects the best available snapshot provider
// Priority: Snapper (if BTRFS and configured) > Timeshift > None
func NewManager(ctx *core.SystemContext) *Manager {
	// 1. Try Snapper
	// Snapper is preferred for BTRFS because of atomic/fast snapshots
	snapper := NewSnapper()
	if ctx.FSInfo.RootFSType == "btrfs" && snapper.IsAvailable(ctx) {
		// Basit bir kontrol: snapper list komutu çalışıyor mu? (Config var mı?)
		// Detaylı kontrolü provider içinde yapmak daha iyi olabilir ama şimdilik availability yeterli.
		return &Manager{provider: snapper}
	}

	// 2. Try Timeshift
	timeshift := NewTimeshift()
	if timeshift.IsAvailable(ctx) {
		return &Manager{provider: timeshift}
	}

	// 3. Fallback to Snapper if configured on non-BTRFS (e.g. LVM thin provisioning support in snapper exists too)
	if snapper.IsAvailable(ctx) {
		return &Manager{provider: snapper}
	}

	return nil
}

func (m *Manager) IsAvailable(ctx *core.SystemContext) bool {
	return m != nil && m.provider != nil && m.provider.IsAvailable(ctx)
}

func (m *Manager) ProviderName() string {
	if m.provider == nil {
		return "None"
	}
	return m.provider.Name()
}

func (m *Manager) CreatePreSnapshot(ctx *core.SystemContext, desc string) (string, error) {
	if m.provider == nil {
		return "", nil
	}
	pterm.Info.Printf("Creating system snapshot using %s...\n", m.provider.Name())
	return m.provider.CreatePreSnapshot(ctx, desc)
}

func (m *Manager) CreatePostSnapshot(ctx *core.SystemContext, id string, desc string) error {
	if m.provider == nil {
		return nil
	}
	return m.provider.CreatePostSnapshot(ctx, id, desc)
}
