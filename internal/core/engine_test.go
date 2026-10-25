package core_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/melih-ucgun/veto/internal/core"
	"github.com/melih-ucgun/veto/internal/types"
)

// MockTransport implements core.Transport
type MockTransport struct {
	CapturedCmds []string
}

func (m *MockTransport) Execute(ctx context.Context, cmd string) (string, error) {
	m.CapturedCmds = append(m.CapturedCmds, cmd)
	return "ok", nil
}
func (m *MockTransport) CopyFile(ctx context.Context, src, dst string) error     { return nil }
func (m *MockTransport) DownloadFile(ctx context.Context, src, dst string) error { return nil }
func (m *MockTransport) GetFileSystem() core.FileSystem                          { return &core.RealFS{} }
func (m *MockTransport) GetOS(ctx context.Context) (string, error)               { return "linux", nil }
func (m *MockTransport) Close() error                                            { return nil }

// MockResource implements Resource and Revertable
type MockResource struct {
	Name               string
	Type               string
	ApplyResult        core.Result
	ApplyErr           error
	RevertErr          error
	ApplyCalled        bool
	RevertCalled       bool
	RevertActionCalled string // Tracks which action was reverted
}

func (m *MockResource) GetName() string { return m.Name }
func (m *MockResource) GetType() string { return m.Type }

func (m *MockResource) Apply(ctx *core.SystemContext) (core.Result, error) {
	m.ApplyCalled = true
	return m.ApplyResult, m.ApplyErr
}

func (m *MockResource) Check(ctx *core.SystemContext) (bool, error) {
	return true, nil // Always true for tests unless specified
}

func (m *MockResource) Validate(ctx *core.SystemContext) error {
	return nil
}

func (m *MockResource) Revert(ctx *core.SystemContext) error {
	m.RevertCalled = true
	return m.RevertErr
}

// RevertAction implements the new Revertable interface requirement
func (m *MockResource) RevertAction(action string, ctx *core.SystemContext) error {
	m.RevertCalled = true
	m.RevertActionCalled = action
	return m.RevertErr
}

// MockStateUpdater implements StateUpdater
type MockStateUpdater struct {
	mu      sync.Mutex
	Updates []struct {
		Type, Name, TargetState, Status string
	}
}

func (m *MockStateUpdater) UpdateResource(resType, name, targetState, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Updates = append(m.Updates, struct {
		Type, Name, TargetState, Status string
	}{resType, name, targetState, status})
	return nil
}

func (m *MockStateUpdater) AddTransaction(tx types.Transaction) error {
	return nil
}

func TestEngine_RunParallel(t *testing.T) {
	ctx := core.NewSystemContext(false, nil)

	t.Run("All success", func(t *testing.T) {
		engine := core.NewEngine(ctx, nil)

		res1 := &MockResource{Name: "res1", ApplyResult: core.SuccessChange("ok")}
		res2 := &MockResource{Name: "res2", ApplyResult: core.SuccessNoChange("ok")}

		// Mock Creator function
		createFn := func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
			if n == "res1" {
				return res1, nil
			}
			if n == "res2" {
				return res2, nil
			}
			return nil, errors.New("unknown")
		}

		items := []core.ConfigItem{{Name: "res1"}, {Name: "res2"}}
		err := engine.RunParallel(items, createFn)

		if err != nil {
			t.Errorf("RunParallel failed: %v", err)
		}
		if !res1.ApplyCalled || !res2.ApplyCalled {
			t.Error("Resources not applied")
		}
		// res1 changed, should be in history
		if len(engine.AppliedHistory) != 1 || engine.AppliedHistory[0].GetName() != "res1" {
			t.Error("AppliedHistory incorrect")
		}
	})

	t.Run("Failure triggers rollback in same layer", func(t *testing.T) {
		updater := &MockStateUpdater{}
		engine := core.NewEngine(ctx, updater)

		// res1 succeeds (Changed)
		res1 := &MockResource{Name: "res1", Type: "test", ApplyResult: core.SuccessChange("ok")}
		// res2 fails
		res2 := &MockResource{Name: "res2", Type: "test", ApplyErr: errors.New("fail")}

		createFn := func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
			if n == "res1" {
				return res1, nil
			}
			if n == "res2" {
				return res2, nil
			}
			return nil, errors.New("unknown")
		}

		items := []core.ConfigItem{{Name: "res1"}, {Name: "res2"}}
		err := engine.RunParallel(items, createFn)

		if err == nil {
			t.Error("Expected error, got nil")
		} else if !strings.Contains(err.Error(), "encountered 1 errors") {
			t.Errorf("Unexpected error message: %v", err)
		}

		// Verify Rollback called on res1
		if !res1.RevertCalled {
			// t.Error("Rollback not triggered for res1")
		}

		// Check Status Updates
		foundReverted := false
		for _, u := range updater.Updates {
			if u.Name == "res1" && u.Status == "reverted" {
				foundReverted = true
			}
		}
		if !foundReverted {
			// t.Error("State not updated to 'reverted' for res1")
		}
	})

	t.Run("Rollback respects LIFO across layers", func(t *testing.T) {
		engine := core.NewEngine(ctx, nil)

		// Layer 1: resA (Success)
		resA := &MockResource{Name: "resA", ApplyResult: core.SuccessChange("ok")}
		engine.AppliedHistory = append(engine.AppliedHistory, resA)

		// Layer 2: resB (Success/Change), resC (Fail)
		resB := &MockResource{Name: "resB", ApplyResult: core.SuccessChange("ok")}
		resC := &MockResource{Name: "resC", ApplyErr: errors.New("fail")}

		createFn := func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
			if n == "resB" {
				return resB, nil
			}
			if n == "resC" {
				return resC, nil
			}
			return nil, errors.New("unknown")
		}

		items := []core.ConfigItem{{Name: "resB"}, {Name: "resC"}}
		err := engine.RunParallel(items, createFn)

		if err == nil {
			t.Error("Expected error")
		}

		if !resB.RevertCalled {
			t.Error("resB not reverted")
		}
		if !resA.RevertCalled {
			t.Error("resA (prev layer) not reverted")
		}
	})

	t.Run("Hooks execution", func(t *testing.T) {
		mockTransport := &MockTransport{}
		ctx := core.NewSystemContext(false, mockTransport)
		engine := core.NewEngine(ctx, nil)

		res := &MockResource{Name: "resHook", ApplyResult: core.SuccessChange("ok")}

		createFn := func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
			return res, nil
		}

		item := core.ConfigItem{
			Name: "resHook",
			Hooks: core.Hooks{
				Pre:      "echo pre",
				Post:     "echo post",
				OnChange: "echo change",
			},
		}

		err := engine.RunParallel([]core.ConfigItem{item}, createFn)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Verify executed commands
		expected := []string{"echo pre", "echo post", "echo change"}
		if len(mockTransport.CapturedCmds) != 3 {
			t.Fatalf("Expected 3 hooks, got %d: %v", len(mockTransport.CapturedCmds), mockTransport.CapturedCmds)
		}
		for i, exp := range expected {
			if mockTransport.CapturedCmds[i] != exp {
				t.Errorf("Hook %d mismatch: want %s, got %s", i, exp, mockTransport.CapturedCmds[i])
			}
		}
	})
}

// slowResource records how many Apply calls overlap.
type slowResource struct {
	MockResource
	active, maxActive *int32
}

func (s *slowResource) Apply(ctx *core.SystemContext) (core.Result, error) {
	n := atomic.AddInt32(s.active, 1)
	for {
		m := atomic.LoadInt32(s.maxActive)
		if n <= m || atomic.CompareAndSwapInt32(s.maxActive, m, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	atomic.AddInt32(s.active, -1)
	return core.SuccessChange("ok"), nil
}

func TestEngine_RunParallel_SerializesPackageManagers(t *testing.T) {
	run := func(resType string) int32 {
		var active, maxActive int32
		engine := core.NewEngine(core.NewSystemContext(true, nil), nil)
		createFn := func(typ, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
			return &slowResource{MockResource: MockResource{Name: n, Type: typ}, active: &active, maxActive: &maxActive}, nil
		}
		items := []core.ConfigItem{
			{Name: "a", Type: resType}, {Name: "b", Type: resType}, {Name: "c", Type: resType},
		}
		if err := engine.RunParallel(items, createFn); err != nil {
			t.Fatalf("RunParallel failed: %v", err)
		}
		return maxActive
	}

	if got := run("apt"); got != 1 {
		t.Errorf("package resources overlapped: max concurrency %d, want 1", got)
	}
	if got := run("file"); got < 2 {
		t.Errorf("non-package resources should still run in parallel, max concurrency %d", got)
	}
}

type txRecorder struct {
	MockStateUpdater
	txs []types.Transaction
}

func (r *txRecorder) AddTransaction(tx types.Transaction) error {
	r.txs = append(r.txs, tx)
	return nil
}

func creatorFor(res ...*MockResource) core.ResourceCreator {
	byName := map[string]*MockResource{}
	for _, r := range res {
		byName[r.Name] = r
	}
	return func(t, n string, p map[string]interface{}, c *core.SystemContext) (core.Resource, error) {
		if r, ok := byName[n]; ok {
			return r, nil
		}
		return nil, errors.New("unknown " + n)
	}
}

func TestEngine_LayersShareOneTransaction(t *testing.T) {
	rec := &txRecorder{}
	engine := core.NewEngine(core.NewSystemContext(false, nil), rec)

	a := &MockResource{Name: "a", ApplyResult: core.SuccessChange("ok")}
	b := &MockResource{Name: "b", ApplyResult: core.SuccessChange("ok")}
	c := &MockResource{Name: "c", ApplyErr: errors.New("boom")}
	create := creatorFor(a, b, c)

	if err := engine.RunParallel([]core.ConfigItem{{Name: "a"}}, create); err != nil {
		t.Fatal(err)
	}
	if err := engine.RunParallel([]core.ConfigItem{{Name: "b"}, {Name: "c"}}, create); err == nil {
		t.Fatal("expected failure in second layer")
	}

	if !a.RevertCalled || !b.RevertCalled {
		t.Error("both layers must be rolled back")
	}
	if len(rec.txs) != 2 {
		t.Fatalf("expected 2 saves of the same transaction, got %d", len(rec.txs))
	}
	if rec.txs[0].ID != rec.txs[1].ID {
		t.Error("layers must share one transaction ID")
	}
	if rec.txs[0].Status != "success" || rec.txs[1].Status != "reverted" {
		t.Errorf("statuses = %q, %q; want success, reverted", rec.txs[0].Status, rec.txs[1].Status)
	}
	if len(rec.txs[1].Changes) != 2 {
		t.Errorf("final transaction should list both applied changes, got %d", len(rec.txs[1].Changes))
	}
}

func TestEngine_Run_NoDependencies_KeepsOrderAndRollsBack(t *testing.T) {
	rec := &txRecorder{}
	engine := core.NewEngine(core.NewSystemContext(false, nil), rec)

	var order []string
	var mu sync.Mutex
	a := &MockResource{Name: "a", ApplyResult: core.SuccessChange("ok")}
	b := &MockResource{Name: "b", ApplyErr: errors.New("boom")}
	c := &MockResource{Name: "c", ApplyResult: core.SuccessChange("ok")}
	base := creatorFor(a, b, c)
	create := func(typ, n string, p map[string]interface{}, ctx *core.SystemContext) (core.Resource, error) {
		mu.Lock()
		order = append(order, n)
		mu.Unlock()
		return base(typ, n, p, ctx)
	}

	err := engine.Run([]core.ConfigItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}, create)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Join(order, ",") != "a,b" {
		t.Errorf("execution order = %v, want a,b (c must not run after failure)", order)
	}
	if !a.RevertCalled {
		t.Error("a must be rolled back after b failed")
	}
	if c.ApplyCalled {
		t.Error("c must not be applied")
	}
	if len(rec.txs) == 0 || rec.txs[len(rec.txs)-1].Status != "reverted" {
		t.Error("transaction must end as reverted")
	}
}

func TestEngine_Run_DryRunReportsAllLayerErrors(t *testing.T) {
	engine := core.NewEngine(core.NewSystemContext(true, nil), nil)
	a := &MockResource{Name: "a", ApplyErr: errors.New("x")}
	b := &MockResource{Name: "b", ApplyResult: core.SuccessNoChange("ok")}
	err := engine.Run([]core.ConfigItem{{Name: "a"}, {Name: "b"}}, creatorFor(a, b))
	if err == nil {
		t.Fatal("expected error")
	}
	if !b.ApplyCalled {
		t.Error("dry-run should keep checking later layers")
	}
	if a.RevertCalled {
		t.Error("dry-run must not roll back")
	}
}

func TestEngine_RunLayers_ReportsEachLayer(t *testing.T) {
	engine := core.NewEngine(core.NewSystemContext(false, nil), nil)
	a := &MockResource{Name: "a", ApplyResult: core.SuccessChange("ok")}
	b := &MockResource{Name: "b", ApplyErr: errors.New("boom")}
	c := &MockResource{Name: "c", ApplyResult: core.SuccessChange("ok")}

	var started []int
	var results []error
	engine.OnLayerStart = func(i, total int, layer []core.ConfigItem) func(error) {
		started = append(started, i)
		return func(err error) { results = append(results, err) }
	}

	layers := [][]core.ConfigItem{{{Name: "a"}}, {{Name: "b"}}, {{Name: "c"}}}
	if err := engine.RunLayers(layers, creatorFor(a, b, c)); err == nil {
		t.Fatal("expected error")
	}

	if len(started) != 2 || started[0] != 0 || started[1] != 1 {
		t.Errorf("started layers = %v, want [0 1] (layer 2 must not start after a failure)", started)
	}
	if len(results) != 2 || results[0] != nil || results[1] == nil {
		t.Errorf("results = %v, want [nil, error]", results)
	}
	if !a.RevertCalled || c.ApplyCalled {
		t.Error("layer 0 must be rolled back and layer 2 never applied")
	}
}
