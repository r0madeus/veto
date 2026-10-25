package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/r0madeus/veto/internal/types"
)

// MockRealFS implements state.FileSystem for tests using real os calls
type MockRealFS struct{}

func (fs *MockRealFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (fs *MockRealFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (fs *MockRealFS) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func TestHistoryManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "veto-history-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "state.json")
	fs := &MockRealFS{}
	mgr, err := NewManager(stateFile, fs)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// 1. Add Transaction
	tx := types.Transaction{
		ID:        "tx1",
		Timestamp: time.Now(),
		Status:    "success",
		Changes: []types.TransactionChange{
			{Type: "pkg", Name: "vim", Action: "installed"},
		},
	}

	if err := mgr.AddTransaction(tx); err != nil {
		t.Fatalf("AddTransaction failed: %v", err)
	}

	// 2. Read Back
	// Create new manager instance to ensure reading from file
	mgr2, err := NewManager(stateFile, fs)
	if err != nil {
		t.Fatal(err)
	}

	txs := mgr2.GetTransactions()
	if len(txs) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(txs))
	}

	if txs[0].ID != "tx1" {
		t.Errorf("Transaction ID mismatch. Got %s, want tx1", txs[0].ID)
	}
	if len(txs[0].Changes) != 1 {
		t.Error("Transaction changes mismatch")
	}
}

func TestAddTransaction_UpsertsByID(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(filepath.Join(dir, "state.json"), &MockRealFS{})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.AddTransaction(types.Transaction{ID: "tx1", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddTransaction(types.Transaction{ID: "tx2", Status: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddTransaction(types.Transaction{ID: "tx1", Status: "reverted"}); err != nil {
		t.Fatal(err)
	}

	txs := mgr.GetTransactions()
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txs))
	}
	if txs[0].ID != "tx1" || txs[0].Status != "reverted" {
		t.Errorf("tx1 should be updated in place, got %+v", txs[0])
	}
}
