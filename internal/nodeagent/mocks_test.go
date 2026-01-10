package nodeagent

import "context"

// mockRunController is a test mock for the RunController interface.
// It tracks method calls and allows configuring return values.
type mockRunController struct {
	startCalled  bool
	stopCalled   bool
	startErr     error
	stopErr      error
	lastStart    StartRunRequest
	lastStop     StopRunRequest
	acquireCalls int
	releaseCalls int

	// slotSem is a mock concurrency semaphore. If nil, AcquireSlot/ReleaseSlot
	// are no-ops. Tests can set this to simulate concurrency limiting.
	slotSem chan struct{}
}

func (m *mockRunController) StartRun(ctx context.Context, req StartRunRequest) error {
	m.startCalled = true
	m.lastStart = req
	return m.startErr
}

func (m *mockRunController) StopRun(ctx context.Context, req StopRunRequest) error {
	m.stopCalled = true
	m.lastStop = req
	return m.stopErr
}

// AcquireSlot implements RunController. If slotSem is set, blocks until a slot
// is available; otherwise returns immediately.
func (m *mockRunController) AcquireSlot(ctx context.Context) error {
	m.acquireCalls++
	if m.slotSem == nil {
		return nil
	}
	select {
	case m.slotSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot implements RunController. If slotSem is set, releases a slot.
func (m *mockRunController) ReleaseSlot() {
	m.releaseCalls++
	if m.slotSem == nil {
		return
	}
	<-m.slotSem
}

// mockController is a minimal no-op RunController implementation for testing.
// Use this when you don't need to track method calls or configure behavior.
type mockController struct{}

func (m *mockController) StartRun(ctx context.Context, req StartRunRequest) error {
	return nil
}

func (m *mockController) StopRun(ctx context.Context, req StopRunRequest) error {
	return nil
}

func (m *mockController) AcquireSlot(ctx context.Context) error {
	return nil
}

func (m *mockController) ReleaseSlot() {}
