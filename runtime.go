package fe

import (
	"errors"
	"fmt"
	"uuid"

	dag "github.com/n24-x/dag-go"
	"github.com/n24-x/fe/eventbus"
	"github.com/n24-x/fe/feconfig"
)

// Runtime is the per-config runtime world of fe: it is built from one
// MachineConfig, lives only as long as that config is active, and is rebuilt
// (a brand-new Runtime) whenever a new config replaces it.
//
// # Concurrency contract
//
// Instances run concurrently once Start completes (their goroutines are
// alive while Stop runs). Two rules keep that safe without locks:
//
//   - Instance goroutines may call [Runtime.Instance], [Runtime.Done] and
//     [Runtime.BusClient] concurrently: after NewRuntime the instances map is
//     never written again, so concurrent reads are race-free, and the Bus
//     guards its own client list. (Instances reach the Runtime only through
//     the narrow [RuntimeAccess] view, which exposes exactly these three
//     methods.)
//   - Start/Stop are NOT safe for concurrent use with each other: they
//     mutate the one-shot lifecycle state. The framework serializes them by
//     construction — the Manager (Apply/Stop, mutex-guarded) or the
//     application's single main goroutine drives the lifecycle; instance
//     goroutines never hold a *Runtime and cannot reach Start/Stop.
//
// TODO(next):
// Three orthogonal channels (see issue.md, review 4):
//   - done: the Runtime's lifecycle signal. Instances that spawn goroutines
//     select on it; cleanup closes it.
//   - Bus: state-change notifications (one-way, no return value).
//   - (a future info container: identity/deps/logger handed to instances).
type Runtime struct {
	// done is the Runtime's lifecycle signal: created in NewRuntime, closed by
	// cleanup. Instance goroutines select on it to learn that the Runtime is
	// going away.
	//
	// It is held bidirectionally but only ever handed out receive-only (see
	// [RuntimeAccess.Done]), so the direction itself stops a module from
	// closing it: the same guarantee the unexported cancel func used to give,
	// now enforced by the type system instead of by convention. Unlike
	// context.Context.Done it is never nil, so selecting on it cannot block
	// forever.
	done chan struct{}

	// bus is the Runtime's state-change notification channel (issue.md §7):
	// built by NewRuntime from the config's Options.Bus, released by cleanup.
	// Instances never touch it directly — they get a client of their own from
	// [Runtime.BusClient].
	bus *eventbus.Bus

	// cfg is the machine config this Runtime was built from (read-only).
	cfg *feconfig.MachineConfig

	// instances are the running instances, keyed by their id.
	instances map[InstanceID]Instance

	// mods is the set of module types used by this Runtime.
	mods map[ModuleID]bool

	// lifecycle is the Runtime's lifecycle bookkeeping.
	//
	// TODO(next):
	// (issue.md D4/D21):
	// the topological instance order plus the one-shot started/stopped
	// flags. Grouped under one field to keep Runtime lean while it is
	// still early.
	lifecycle lifecycle
}

// RuntimeAccess is the module-visible view of the Runtime handed to a
// Provision call: resolve a dependency instance, observe the lifecycle
// signal, open a Bus client. It deliberately hides the rest of the Runtime
// (lifecycle, registry, …) and sits next to [Runtime], its only real
// implementation.
//
// Modules never receive a *Runtime: the framework hands out the sealed
// [moduleView] instead, so the hidden part stays hidden even against a type
// assertion. Tests may fake the interface.
type RuntimeAccess interface {
	// Instance resolves a dependency instance by its config id.
	Instance(id string) (inst Instance, err error)
	// Done returns the Runtime's lifecycle signal, receive-only: it is closed
	// when the Runtime goes away (Stop, a failed construction, a rolled-back
	// Start). It is never nil, so a select on it cannot block forever.
	Done() (done <-chan struct{})
	// BusClient opens a Bus client for the calling instance. name is a label
	// for a human reading debug logs and client-scoped errors; it is NOT
	// required to be unique — the framework does not route by it.
	BusClient(name string) (client *eventbus.Client, err error)
}

// moduleView is the sealed [RuntimeAccess] the framework hands to modules: it
// forwards exactly the interface's three methods and adds nothing.
//
// Handing over *Runtime would satisfy the interface without sealing it. An
// interface value carries its dynamic type, so a module — which lives in a
// package of its own — could recover the concrete Runtime with a plain type
// assertion and call [Runtime.Start] / [Runtime.Stop], driving the lifecycle
// the framework owns (D30). moduleView's method set is exactly the
// interface's, and its name is unexported, so such an assertion finds nothing.
type moduleView struct{ r *Runtime }

func (v moduleView) Instance(id string) (Instance, error) { return v.r.Instance(id) }
func (v moduleView) Done() <-chan struct{}                { return v.r.Done() }

func (v moduleView) BusClient(name string) (*eventbus.Client, error) {
	return v.r.BusClient(name)
}

// lifecycle is the Runtime's lifecycle state machine (issue.md D4/D21):
// created → started → stopped.
type lifecycle struct {
	// startOrder is the order in which instances were created during
	// NewRuntime (topological: deps first), recorded as instance ids. It
	// is fixed at construction. Start walks it forward; Stop walks it in
	// reverse (D4).
	startOrder []InstanceID

	// started reports that Start completed successfully: every instance
	// is running.
	started bool
	// stopped reports that the Runtime reached its terminal state: Stop
	// was called, or a Start attempt failed and was rolled back. A stopped
	// Runtime cannot be started again.
	stopped bool
}

// ValidateRuntimeConfig performs runtime-level semantic validation of a
// machine config: every instance's mod_id must be registered AND produce
// instances (implement Provisioner).
//
// TODO(next)
// This is flow step [2] (see cmd/main.go).
// Syntax/structure validation is feconfig.MachineConfigValidate's job.
//
// It is a pure check with no side effects; NewRuntime calls it first.
func ValidateRuntimeConfig(mc *feconfig.MachineConfig) error {
	for i, inst := range mc.Instances {
		m, err := GetModule(ModuleID(inst.ModuleID))
		if err != nil {
			return fmt.Errorf("fe: validate runtime config: instances[%d].mod_id %q: %w", i, inst.ModuleID, err)
		}
		if _, ok := m.(Provisioner); !ok {
			return fmt.Errorf("fe: validate runtime config: instances[%d].mod_id %q: %w", i, inst.ModuleID, ErrModuleNotProvisioner)
		}
	}
	return nil
}

// NewRuntime builds a Runtime from a machine config.
//
// Pipeline implemented so far (see issue.md §4, flow steps [2]-[3]):
//
//	ValidateRuntimeConfig (semantic validation)
//	→ resolve the instance creation order via the dependency graph
//	→ instantiate every instance in that order (Provision), recording the
//	  creation order in lifecycle.startOrder
//
// The returned Runtime is NOT started: instances are created but idle. The
// caller starts them with Start (flow step [4]), or discards the Runtime
// with Stop, which releases its lifecycle signal and Bus without starting
// anything.
//
// On failure NewRuntime returns no Runtime for the caller to Stop, so every
// error path below cleans up after itself before returning (see cleanup).
func NewRuntime(mc *feconfig.MachineConfig) (*Runtime, error) {
	if err := ValidateRuntimeConfig(mc); err != nil {
		return nil, err
	}

	order, err := instanceOrder(mc)
	if err != nil {
		return nil, fmt.Errorf("fe: new runtime: resolving instance order: %w", err)
	}

	r := &Runtime{
		done:      make(chan struct{}),
		cfg:       mc,
		bus:       eventbus.NewWithOptions(mc.Options.Bus),
		instances: make(map[InstanceID]Instance, len(order)),
		mods:      make(map[ModuleID]bool, len(order)),
		lifecycle: lifecycle{startOrder: make([]InstanceID, 0, len(order))},
	}

	for _, spec := range order {
		id, err := uuid.Parse(spec.InstanceID)
		if err != nil {
			r.cleanup() // construction failed: nothing started, so close signal + Bus
			return nil, fmt.Errorf("fe: new runtime: invalid instance id %q: %w", spec.InstanceID, err)
		}

		inst, err := r.provision(*spec)
		if err != nil {
			r.cleanup()
			return nil, fmt.Errorf("fe: new runtime: instance %q (%s): %w", spec.InstanceID, spec.ModuleID, err)
		}
		if inst == nil {
			r.cleanup()
			return nil, fmt.Errorf("fe: new runtime: instance %q (%s): module returned a nil instance", spec.InstanceID, spec.ModuleID)
		}

		instID := InstanceID(id)
		r.instances[instID] = inst
		r.mods[ModuleID(spec.ModuleID)] = true
		r.lifecycle.startOrder = append(r.lifecycle.startOrder, instID)
	}

	return r, nil
}

// instanceOrder returns the order in which instances must be created: every
// instance after all of its deps (deps first).
//
// Every spec is a dag node: it Provides its own instance id and Requires its
// deps' ids (see feconfig.InstanceSpec.Requires/Provides). The "root" specs
// to resolve are the terminal consumers — specs whose id no other spec
// depends on. Resolving each terminal consumer yields its whole dependency
// chain, deps-first (DFS post-order); the merged result covers every spec.
func instanceOrder(mc *feconfig.MachineConfig) ([]*feconfig.InstanceSpec, error) {
	g := dag.New[string]()
	for i := range mc.Instances {
		if _, _, err := g.Add(&mc.Instances[i]); err != nil {
			return nil, fmt.Errorf("cycle in instance deps: %w", err)
		}
	}

	// terminal consumers = ids that appear in no other spec's deps
	dependedOn := make(map[string]bool, len(mc.Instances))
	for _, s := range mc.Instances {
		for _, d := range s.Deps {
			dependedOn[d] = true
		}
	}
	var targets []string
	for _, s := range mc.Instances {
		if !dependedOn[s.InstanceID] {
			targets = append(targets, s.InstanceID)
		}
	}

	// Resolve each terminal consumer, merging (deduplicating) the order.
	// Resolve returns deps before dependents.
	seen := make(map[string]bool, len(mc.Instances))
	order := make([]*feconfig.InstanceSpec, 0, len(mc.Instances))
	for _, target := range targets {
		seq, err := dag.Resolve(g, target)
		if err != nil {
			return nil, err
		}
		for _, nid := range seq {
			spec := g.At(nid).(*feconfig.InstanceSpec)
			if !seen[spec.InstanceID] {
				seen[spec.InstanceID] = true
				order = append(order, spec)
			}
		}
	}
	return order, nil
}

// provision constructs one instance from a spec by delegating to the
// module's Provisioner. Config parsing is entirely the module author's job
// (the framework does not decode spec.Config — see issue.md func.md).
// GetModule returning a module that ValidateRuntimeConfig accepted as a
// Provisioner guarantees the type assertion below succeeds.
//
// The module receives a [moduleView], never the Runtime itself: see that type
// for why the distinction matters.
func (r *Runtime) provision(spec feconfig.InstanceSpec) (Instance, error) {
	m, err := GetModule(ModuleID(spec.ModuleID))
	if err != nil {
		return nil, err
	}
	return m.(Provisioner).Provision(spec, moduleView{r: r})
}

// Start starts the Runtime: it transitions into the Running state (flow
// step [4]) by starting every instance in creation order — deps first,
// matching the order recorded in lifecycle.startOrder (D4).
//
// Start is one-shot and failure-atomic (D3):
//   - if an instance fails to start, every instance that already started is
//     stopped again, in reverse start order, and the error is returned. The
//     failing instance itself is not stopped — it never reached Started
//     (mirrors caddy, whose Start-failure rollback stops only the started
//     apps); closing the lifecycle signal tells any goroutines it may have
//     spawned to wind down (there is no resource layer to release — D25).
//   - after a failed Start the Runtime is left stopped/defunct: the lifecycle
//     signal is closed, and Start refuses to run again.
//   - starting an already-started (or already-stopped) Runtime is an error.
//
// Not safe for concurrent use with Stop.
func (r *Runtime) Start() error {
	switch {
	case r.lifecycle.stopped:
		return errors.New("fe: runtime stopped; cannot be started")
	case r.lifecycle.started:
		return errors.New("fe: runtime already started")
	}

	// Rollback buffer: instances started so far, stopped in reverse on error.
	started := make([]InstanceID, 0, len(r.lifecycle.startOrder))
	for _, id := range r.lifecycle.startOrder {
		if err := r.instances[id].Start(); err != nil {
			var rollbackErr error
			for i := len(started) - 1; i >= 0; i-- {
				stopID := started[i]
				if err2 := r.instances[stopID].Stop(); err2 != nil {
					rollbackErr = errors.Join(rollbackErr,
						fmt.Errorf("fe: start: rollback stop of instance %s: %w", stopID, err2))
				}
			}
			// Rollback done: the Runtime is defunct (D3), so release the
			// signal and the Bus; Start refuses to run again.
			r.lifecycle.stopped = true
			r.cleanup()
			if rollbackErr != nil {
				return errors.Join(fmt.Errorf("fe: start: instance %s: %w", id, err), rollbackErr)
			}
			return fmt.Errorf("fe: start: instance %s: %w", id, err)
		}
		started = append(started, id)
	}

	r.lifecycle.started = true
	return nil
}

// Stop shuts the Runtime down: it stops every started instance in reverse
// start order, then cleans up — closes the lifecycle signal so instance
// goroutines wind down, and releases the Bus.
//
// Rules:
//   - instances that were provisioned but never started (Stop before Start,
//     or a rolled-back Start) are not stopped — they never began. There is
//     nothing to release for them either: by contract (D25) Provision is
//     side-effect-free (config parsing + dependency capture only), so all
//     resources are Start/Stop-scoped; closing the signal still tells any
//     goroutines they may have spawned that the Runtime is gone.
//   - Stop is idempotent: it may be called any number of times, before or
//     after Start, and after a failed Start. Only the first call releases
//     anything — the ones after it return early, which is also what keeps the
//     lifecycle signal from being closed twice.
//   - errors from individual Stop calls are aggregated with errors.Join.
//
// Not safe for concurrent use with Start or with another Stop: the early
// return above is the only thing standing between a second call and closing
// an already-closed channel, so callers must serialize (the Manager does,
// under its mutex).
func (r *Runtime) Stop() error {
	if r.lifecycle.stopped {
		return nil
	}

	var err error
	if r.lifecycle.started {
		for i := len(r.lifecycle.startOrder) - 1; i >= 0; i-- {
			id := r.lifecycle.startOrder[i]
			if err2 := r.instances[id].Stop(); err2 != nil {
				err = errors.Join(err, fmt.Errorf("fe: stop: instance %s: %w", id, err2))
			}
		}
		r.lifecycle.started = false
	}
	r.lifecycle.stopped = true
	r.cleanup()
	return err
}

// cleanup releases the framework-owned resources of a Runtime: it closes the
// lifecycle signal (waking every instance goroutine that selects on it) and
// closes the Bus (which closes every client it handed out).
//
// It must run exactly once per Runtime — unlike the cancel func it replaces,
// closing an already-closed channel panics. No sync.Once is needed because the
// callers already guarantee it: NewRuntime's failure paths return nil, so that
// Runtime reaches nobody else, and both Start's rollback and Stop mark the
// Runtime stopped, which is the guard Stop checks.
//
// It is the teardown of everything NewRuntime acquires, so it serves both
// kinds of caller: NewRuntime itself, on every construction failure — the
// caller gets no Runtime to Stop — and Stop, as its last step, once the
// instances have been stopped.
func (r *Runtime) cleanup() {
	close(r.done)
	r.bus.Close()
}

// Instance returns the instance whose config id is id, or an error if no
// such instance exists in this Runtime. id is the raw uuid string as it
// appears in the MachineConfig — the string form a module stores when its
// config references a dependency (D11).
//
// Instances become visible only once they have finished Provision (D6), and
// NewRuntime provisions deps first; so Instance is usable from inside a
// Provision call to resolve the instance's own dependencies, and at any
// later point while the Runtime lives.
func (r *Runtime) Instance(id string) (Instance, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("fe: instance %q: invalid id: %w", id, err)
	}
	inst, ok := r.instances[InstanceID(parsed)]
	if !ok {
		return nil, fmt.Errorf("%w: %s (declared in the referencing instance's deps? see issue.md D11)", ErrInstanceNotFound, id)
	}
	return inst, nil
}

// Done returns the Runtime's lifecycle signal, receive-only. Instance
// goroutines select on it to know when to stop; Stop, a failed construction
// and a rolled-back Start all close it.
//
// Receive-only is deliberate: a module cannot close a channel it only receives
// from, so one instance cannot take the whole Runtime down — the rule the
// unexported cancel func used to encode, now enforced by the type system.
func (r *Runtime) Done() <-chan struct{} {
	return r.done
}

// BusClient opens a new Bus client named name, for the calling instance to
// keep for as long as it lives. Every call returns a fresh client; the caller
// decides how to share it (typically by storing it once, in Provision).
//
// name is a label for humans only — it surfaces in [eventbus.Client.Name] and
// in client-scoped errors (ErrSubscriberExists, ErrPublisherExists) — and does
// NOT have to be unique: nothing in the framework routes by it, so passing the
// instance id is a convention, not a requirement.
//
// Ownership stays with the caller: close the client once the instance is done
// with it. Closing is not mandatory, though — the Bus keeps the clients it
// handed out and closes any still open when it is closed, and Client.Close is
// idempotent, so closing twice is harmless.
//
// Once the Runtime has been stopped (or its construction failed) the Bus is
// closed, so this returns eventbus.ErrBusClosed instead of a usable client.
func (r *Runtime) BusClient(name string) (client *eventbus.Client, err error) {
	return r.bus.NewClient(name)
}

// moduleView is what Provision receives; *Runtime is deliberately not handed
// out (see that type). The delegation above keeps the two in step: renaming a
// Runtime method breaks this assertion.
var _ RuntimeAccess = moduleView{}
