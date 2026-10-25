package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/melih-ucgun/veto/internal/types"
)

// FileSystem defines minimum operations required for storage.
// This interface matches core/fs.FileSystem methods used here.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// Manager manages reading/writing the state file.
// It uses a Mutex for thread-safety.
type Manager struct {
	FilePath string
	Current  *types.State
	FS       FileSystem
	mu       sync.RWMutex
}

// NewManager creates a new state manager and loads the existing file.
func NewManager(path string, fs FileSystem) (*Manager, error) {
	mgr := &Manager{
		FilePath: path,
		Current:  types.NewState(),
		FS:       fs,
	}

	// Load existing file if present
	// A missing (or unreadable) state file is not fatal: start with a fresh state.
	_ = mgr.Load()

	return mgr, nil
}

// Load reads state file from abstract FS.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := m.FS.ReadFile(m.FilePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, m.Current)
}

// Save writes current state to abstract FS.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.Current.LastRun = time.Now()

	data, err := json.MarshalIndent(m.Current, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(m.FilePath)
	if err := m.FS.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return m.FS.WriteFile(m.FilePath, data, 0644)
}

// UpdateResource updates a specific resource state and saves it.
func (m *Manager) UpdateResource(resType, name, targetState, status string) error {
	m.mu.Lock()
	id := fmt.Sprintf("%s:%s", resType, name)

	entry := types.ResourceEntry{
		ID:          id,
		Name:        name,
		Type:        resType,
		State:       targetState,
		Status:      status,
		LastApplied: time.Now(),
		Metadata:    make(map[string]interface{}),
	}
	m.Current.Resources[id] = entry
	m.mu.Unlock()

	// Safe to save on every update
	return m.Save()
}
