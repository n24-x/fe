package fe

import (
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n24-x/fe/feconfig"
)

// isClosed reports whether ch has been closed (non-blocking probe).
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Well-known instance ids for the three-instance dependency chain used by
// the lifecycle tests: leaf → mid → top.
const (
	testChainLeafID = "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e" // leaf: depends on nothing
	testChainMidID  = "7c1b4a6e-8f0d-4c9e-9a2b-1c3d4e5f6a7b" // mid: depends on leaf
	testChainTopID  = "5a6b7c8d-9e0f-4a1b-8c2d-3e4f5a6b7c8d" // top: depends on mid
)

// lifecycleTrace records instance lifecycle events in call order.
type lifecycleTrace struct {
	events []string
}

func (t *lifecycleTrace) add(ev string) { t.events = append(t.events, ev) }

func (t *lifecycleTrace) got() string { return strings.Join(t.events, " ") }

// traceInstance is an Instance that records Start/Stop into a shared trace.
// With startErr set, Start fails before recording anything; with stopErr
// set, Stop fails before recording anything.
type traceInstance struct {
	name     string
	trace    *lifecycleTrace
	startErr error
	stopErr  error
}

func (t *traceInstance) Start() error {
	if t.startErr != nil {
		return t.startErr
	}
	t.trace.add("start:" + t.name)
	return nil
}

func (t *traceInstance) Stop() error {
	if t.stopErr != nil {
		return t.stopErr
	}
	t.trace.add("stop:" + t.name)
	return nil
}

// registerChain registers a module producing named trace instances ("a" for
// leaf, "b" for mid, "c" for top) sharing one trace. The instance whose id
// equals failID fails to start with failErr.
func registerChain(t *testing.T, modID ModuleID, trace *lifecycleTrace, failID string, failErr error) {
	t.Helper()
	names := map[string]string{
		testChainLeafID: "a",
		testChainMidID:  "b",
		testChainTopID:  "c",
	}
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		var se error
		if spec.InstanceID == failID {
			se = failErr
		}
		return &traceInstance{name: names[spec.InstanceID], trace: trace, startErr: se}, nil
	}))
}

// TestRuntimeStartStopOrder verifies Start runs instances in creation order
// (deps first) and Stop in reverse, regardless of the config's list order.
func TestRuntimeStartStopOrder(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.order")
	trace := new(lifecycleTrace)
	registerChain(t, modID, trace, "", nil)

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainTopID, ModuleID: string(modID), Deps: []string{testChainMidID}},
			{InstanceID: testChainMidID, ModuleID: string(modID), Deps: []string{testChainLeafID}},
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if want := "start:a start:b start:c"; trace.got() != want {
		t.Fatalf("after Start, trace = %q, want %q", trace.got(), want)
	}
	if isClosed(rt.Done()) {
		t.Fatal("lifecycle signal closed while running")
	}

	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if want := "start:a start:b start:c stop:c stop:b stop:a"; trace.got() != want {
		t.Fatalf("after Stop, trace = %q, want %q", trace.got(), want)
	}
	if !isClosed(rt.Done()) {
		t.Fatal("after Stop, the lifecycle signal is not closed")
	}
}

// TestRuntimeStartFailureRollsBack verifies a Start failure stops the
// already-started instances in reverse order (D3), closes the lifecycle
// signal, and leaves the Runtime defunct (Stop is a no-op, Start refuses
// again).
func TestRuntimeStartFailureRollsBack(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.rollback")
	boom := errors.New("boom")
	trace := new(lifecycleTrace)
	registerChain(t, modID, trace, testChainMidID, boom)

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainTopID, ModuleID: string(modID), Deps: []string{testChainMidID}},
			{InstanceID: testChainMidID, ModuleID: string(modID), Deps: []string{testChainLeafID}},
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime (provisioning must succeed): %v", err)
	}

	err = rt.Start()
	if err == nil {
		t.Fatal("Start: expected error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want errors.Is(err, boom)", err)
	}
	// a started before b failed → stopped again (D3 rollback); c never
	// started; b never recorded a start.
	if want := "start:a stop:a"; trace.got() != want {
		t.Fatalf("after failed Start, trace = %q, want %q", trace.got(), want)
	}
	if !isClosed(rt.Done()) {
		t.Fatal("after failed Start, the lifecycle signal is not closed")
	}

	// The Runtime is defunct: Stop must not re-stop a, and Start must refuse.
	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop after failed Start: %v", err)
	}
	if want := "start:a stop:a"; trace.got() != want {
		t.Fatalf("Stop after failed Start changed trace: %q, want %q", trace.got(), want)
	}
	if err := rt.Start(); err == nil {
		t.Fatal("Start after failed Start: expected error")
	}
}

// TestRuntimeStartStopGuards verifies double Start is an error, Stop is
// idempotent, and a stopped Runtime cannot be started again.
func TestRuntimeStartStopGuards(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.guards")
	trace := new(lifecycleTrace)
	registerChain(t, modID, trace, "", nil)

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := rt.Start(); err == nil {
		t.Fatal("double Start: expected error")
	}
	if want := "start:a"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}

	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := rt.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if want := "start:a stop:a"; trace.got() != want {
		t.Fatalf("trace after double Stop = %q, want %q", trace.got(), want)
	}
	if err := rt.Start(); err == nil {
		t.Fatal("Start after Stop: expected error")
	}
}

// TestRuntimeStopBeforeStart verifies Stop on a never-started Runtime only
// closes the lifecycle signal and does not stop provisioned-but-never-started
// instances.
func TestRuntimeStopBeforeStart(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.stopbefore")
	trace := new(lifecycleTrace)
	registerChain(t, modID, trace, "", nil)

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if trace.got() != "" {
		t.Fatalf("Stop before Start stopped instances: trace = %q", trace.got())
	}
	if !isClosed(rt.Done()) {
		t.Fatal("Stop before Start left the lifecycle signal open")
	}
	if err := rt.Start(); err == nil {
		t.Fatal("Start after Stop: expected error")
	}
}

// TestRuntimeInstanceLookup verifies Instance resolves present ids, and
// reports ErrInstanceNotFound (not a parse error) for absent ones.
func TestRuntimeInstanceLookup(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.lookup")
	RegisterModule(provMod(modID, nil)) // default: fresh fakeInstance per Provision

	absent := "3f2504e0-4f89-41d3-9a0c-0305e82c3301" // well-formed uuid, not in config
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
			{InstanceID: testChainMidID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	for _, id := range []string{testChainLeafID, testChainMidID} {
		if _, err := rt.Instance(id); err != nil {
			t.Fatalf("Instance(%q): %v", id, err)
		}
	}

	_, err = rt.Instance(absent)
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("Instance(%q) error = %v, want errors.Is(err, ErrInstanceNotFound)", absent, err)
	}

	_, err = rt.Instance("not-a-uuid")
	if err == nil {
		t.Fatal("Instance(bad id): expected error")
	}
	if errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("Instance(bad id) error = %v, want an invalid-id error, not ErrInstanceNotFound", err)
	}
}

// registerChainStopFail is registerChain plus a Stop failure on the instance
// whose id equals stopFailID.
func registerChainStopFail(t *testing.T, modID ModuleID, trace *lifecycleTrace, stopFailID string, stopErr error) {
	t.Helper()
	names := map[string]string{
		testChainLeafID: "a",
		testChainMidID:  "b",
		testChainTopID:  "c",
	}
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		var se error
		if spec.InstanceID == stopFailID {
			se = stopErr
		}
		return &traceInstance{name: names[spec.InstanceID], trace: trace, stopErr: se}, nil
	}))
}

// TestRuntimeStopAggregatesErrors verifies Stop stops every started instance
// even when one of them fails, and returns the failure via errors.Is.
func TestRuntimeStopAggregatesErrors(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.stopagg")
	boom := errors.New("stop boom")
	trace := new(lifecycleTrace)
	registerChainStopFail(t, modID, trace, testChainMidID, boom) // b fails to stop

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainTopID, ModuleID: string(modID), Deps: []string{testChainMidID}},
			{InstanceID: testChainMidID, ModuleID: string(modID), Deps: []string{testChainLeafID}},
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	err = rt.Stop()
	if err == nil {
		t.Fatal("Stop: expected error from failing instance")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Stop error = %v, want errors.Is(err, boom)", err)
	}
	// c and a were still stopped despite b failing mid-sequence.
	if want := "start:a start:b start:c stop:c stop:a"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
}

// TestRuntimeStartRollbackStopError verifies that when a Start failure
// triggers a rollback and a rollback Stop also fails, the returned error
// aggregates both the start error and the rollback error, and the Runtime is
// left defunct.
func TestRuntimeStartRollbackStopError(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.rollbackstop")
	startBoom := errors.New("start boom")
	stopBoom := errors.New("rollback stop boom")
	trace := new(lifecycleTrace)
	// b fails to start; a (already started) fails to stop during rollback.
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		inst := &traceInstance{name: map[string]string{
			testChainLeafID: "a",
			testChainMidID:  "b",
			testChainTopID:  "c",
		}[spec.InstanceID], trace: trace}
		switch spec.InstanceID {
		case testChainMidID:
			inst.startErr = startBoom
		case testChainLeafID:
			inst.stopErr = stopBoom
		}
		return inst, nil
	}))

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainTopID, ModuleID: string(modID), Deps: []string{testChainMidID}},
			{InstanceID: testChainMidID, ModuleID: string(modID), Deps: []string{testChainLeafID}},
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}

	err = rt.Start()
	if err == nil {
		t.Fatal("Start: expected error")
	}
	if !errors.Is(err, startBoom) {
		t.Fatalf("Start error = %v, want errors.Is(err, startBoom)", err)
	}
	if !errors.Is(err, stopBoom) {
		t.Fatalf("Start error = %v, want errors.Is(err, stopBoom) (rollback Stop failure joined)", err)
	}
	if !isClosed(rt.Done()) {
		t.Fatal("the lifecycle signal must be closed after a failed Start with a rollback error")
	}
	if err := rt.Stop(); err != nil {
		t.Fatalf("Stop after failed Start: %v", err)
	}
}

// TestRuntimeConcurrentReadAccess verifies the read-only concurrency contract:
// while the Runtime is running (or stopping), instance goroutines may call
// Instance() and Done() concurrently without locking — after NewRuntime
// the instances map is never written again. Run with -race to catch a
// violation.
func TestRuntimeConcurrentReadAccess(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.concurrent")
	RegisterModule(provMod(modID, nil))

	ids := []string{testChainLeafID, testChainMidID, testChainTopID}
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: testChainTopID, ModuleID: string(modID), Deps: []string{testChainMidID}},
			{InstanceID: testChainMidID, ModuleID: string(modID), Deps: []string{testChainLeafID}},
			{InstanceID: testChainLeafID, ModuleID: string(modID)},
		},
	}
	rt, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Hammer Instance()/Done() from many goroutines while the Runtime is
	// running; Stop concurrently on top (also exercises the read path during
	// teardown). -race must report nothing.
	stopDone := make(chan struct{})
	go func() {
		rt.Stop()
		close(stopDone)
	}()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				for _, id := range ids {
					if _, err := rt.Instance(id); err != nil {
						t.Errorf("concurrent Instance(%q): %v", id, err)
						return
					}
				}
				rt.Done() // read-only; must never race
			}
		}()
	}
	wg.Wait()
	<-stopDone
}

// settleGoroutines waits for the goroutine count to drop to at most want and
// returns the count it settled on. Releasing a Runtime reaps its goroutines
// synchronously, so this normally returns on the first poll; the loop absorbs
// unrelated runtime goroutines and keeps a not-yet-reaped one from failing a
// test.
func settleGoroutines(want int) int {
	n := runtime.NumGoroutine()
	for range 200 {
		if n <= want {
			return n
		}
		time.Sleep(5 * time.Millisecond)
		runtime.GC()
		n = runtime.NumGoroutine()
	}
	return n
}

// TestRuntimeBusReleasedOnStop verifies Stop releases the Runtime's Bus.
// Building a Bus starts a router goroutine, and NewRuntime builds one for
// every Runtime, so a missing release shows up as a leak: without it every
// cycle below strands one goroutine for the rest of the process.
func TestRuntimeBusReleasedOnStop(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.busstop")
	RegisterModule(provMod(modID, nil))

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{{InstanceID: testChainLeafID, ModuleID: string(modID)}},
	}

	baseline := runtime.NumGoroutine()
	const cycles = 20
	for range cycles {
		rt, err := NewRuntime(mc)
		if err != nil {
			t.Fatalf("NewRuntime: %v", err)
		}
		if err := rt.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}
	if n := settleGoroutines(baseline); n > baseline {
		t.Fatalf("goroutines after %d NewRuntime+Stop cycles = %d, want <= %d: the Bus was not released", cycles, n, baseline)
	}
}

// TestRuntimeBusReleasedOnConstructionFailure verifies NewRuntime releases the
// Bus it already built when provisioning fails: the caller gets no Runtime, so
// nothing else could release it.
func TestRuntimeBusReleasedOnConstructionFailure(t *testing.T) {
	const modID = ModuleID("fe.test.lifecycle.busfail")
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, errors.New("module rejects the config")
	}))

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{{InstanceID: testChainLeafID, ModuleID: string(modID)}},
	}

	baseline := runtime.NumGoroutine()
	const attempts = 20
	for range attempts {
		if _, err := NewRuntime(mc); err == nil {
			t.Fatal("NewRuntime: expected an error from Provision")
		}
	}
	if n := settleGoroutines(baseline); n > baseline {
		t.Fatalf("goroutines after %d failed NewRuntime = %d, want <= %d: the Bus was not released", attempts, n, baseline)
	}
}
