package fe

import (
	"errors"
	"fmt"
	"log/slog"
	"uuid"

	dag "github.com/n24-x/dag-go"
	"github.com/n24-x/fe/eventbus"
	"github.com/n24-x/fe/feconfig"
)

// Runtime is the per-config runtime world of fe: it is built from one
// [feconfig.MachineConfig] and ended by [Runtime.Stop].
//
// # Concurrency contract
//
// Runtime has two distinct concurrency boundaries: the RuntimeAccess view
// exposed to instance goroutines and the framework lifecycle.
//
// A. [RuntimeAccess] (for module author)
//
// Instance goroutines may call [Runtime.Instance], [Runtime.Done], and
// [Runtime.BusClient] concurrently.
//
// After [NewRuntime], the instances map is never modified, so concurrent reads
// are race-free. The Bus also protects its own client list.
//
// Instance goroutines cannot reach Start or Stop: they receive only the narrow
// [RuntimeAccess] view.
//
// B. Framework lifecycle
//
// Start and Stop are not safe for concurrent use with each other. They mutate
// the one-shot lifecycle state and must be serialized by the framework.
// The App ([App.Start]/[App.Stop]) or the application's single main goroutine
// drives the lifecycle.
type Runtime struct {
	// instances are the running instances, keyed by their id.
	instances map[InstanceID]Instance

	// mods is the set of module types used by this Runtime.
	mods map[ModuleID]bool

	// lifecycle tracks the instance order and one-shot lifecycle state.
	lifecycle lifecycle

	// done is the Runtime's lifecycle signal: created in [NewRuntime] and closed
	// by cleanup(). Instance goroutines use it to detect that the Runtime is
	// going away.
	done chan struct{}

	// bus is the Runtime's state-change notification channel: built by [NewRuntime]
	// from the config's [feconfig.Options.Bus], released by cleanup(). Instances
	// never touch it directly — they get a client of their own from [Runtime.BusClient].
	bus *eventbus.Bus

	// log records this Runtime's lifecycle. It is snapshotted from
	// mc.Options.Logger at construction; nil becomes a discarding logger.
	log *slog.Logger
}

// RuntimeAccess is the module-visible view of the Runtime: resolve dependency
// instances, watch for the Runtime going away, open Bus clients. Most resources
// required by the instance are designed to be provided by other instances;
// for example, logging functionality is provided by the logging instance.
//
// The interface also makes instances easier to test.
type RuntimeAccess interface {
	// Instance resolves a dependency instance by its config id.
	Instance(id string) (inst Instance, err error)

	// Done returns the Runtime's lifecycle signal: closed when the Runtime goes
	// away, and never nil.
	Done() (done <-chan struct{})

	// BusClient opens a Bus client for the calling instance. name is a
	// human-readable label for debug logs and client-scoped errors; it is not
	// required to be unique. Once the Runtime has been stopped the Bus is closed,
	// so this returns [eventbus.ErrBusClosed].
	BusClient(name string) (client *eventbus.Client, err error)
}

// moduleView is the sealed [RuntimeAccess] the framework hands to instances,
// so that they cannot access [*Runtime] or control its lifecycle.
type moduleView struct{ r *Runtime }

var _ RuntimeAccess = moduleView{}

func (v moduleView) Instance(id string) (Instance, error) { return v.r.Instance(id) }
func (v moduleView) Done() <-chan struct{}                { return v.r.Done() }
func (v moduleView) BusClient(name string) (*eventbus.Client, error) {
	return v.r.BusClient(name)
}

// lifecycle is the Runtime's lifecycle state machine:
//
//	created → started → stopped
//	created → stopped            (a failed Start, or Stop before Start)
type lifecycle struct {
	// startOrder is the order in which instances were created during
	// [NewRuntime] (topological order, dependencies first), recorded as
	// instance ids. It is fixed at construction. [Runtime.Start] walks it
	// forward; [Runtime.Stop] walks it in reverse.
	startOrder []InstanceID

	// started reports that the Runtime is started: every instance is running.
	// [Runtime.Stop] resets it, after using it to decide whether there is
	// anything to stop.
	started bool
	// stopped reports that the Runtime reached its terminal state:
	// [Runtime.Stop] was called, or a [Runtime.Start] attempt failed and was
	// rolled back. A stopped Runtime cannot be started again.
	stopped bool
}

// ValidateRuntimeConfig performs runtime-level semantic validation of a
// machine config: every instance's mod_id must be registered AND produce
// instances (implement Provisioner). Syntax and structure are
// [feconfig.MachineConfigValidate]'s job; this is the runtime-level half
// (issue.md §4).
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
// Pipeline implemented so far (issue.md §4; internal/testing/demo/main.go
// walks the stages):
//
//	ValidateRuntimeConfig (semantic validation)
//	→ resolve the instance creation order via the dependency graph
//	→ instantiate every instance in that order (Provision), recording the
//	  creation order in lifecycle.startOrder
//
// The returned Runtime is NOT started: instances are created but idle. The
// caller starts them with Start, or discards the Runtime with Stop, which
// releases its lifecycle signal and Bus without starting anything.
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
		instances: make(map[InstanceID]Instance, len(order)),
		mods:      make(map[ModuleID]bool, len(order)),
		lifecycle: lifecycle{startOrder: make([]InstanceID, 0, len(order))},
		done:      make(chan struct{}),
		bus:       eventbus.NewWithOptions(mc.Options.Bus),
		log:       ensureLogger(mc.Options.Logger),
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

	r.log.Info("runtime created", "instances", len(r.instances))
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
// (the framework does not decode spec.Config — issue.md D20).
// GetModule returning a module that ValidateRuntimeConfig accepted as a
// Provisioner guarantees the type assertion below succeeds.
func (r *Runtime) provision(spec feconfig.InstanceSpec) (Instance, error) {
	m, err := GetModule(ModuleID(spec.ModuleID))
	if err != nil {
		return nil, err
	}
	return m.(Provisioner).Provision(spec, moduleView{r: r})
}

// Start starts the Runtime: it transitions into the Running state by starting
// every instance in creation order — deps first, matching the order recorded
// in lifecycle.startOrder (D4).
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
	r.log.Info("runtime starting")
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
			startErr := fmt.Errorf("fe: start: instance %s: %w", id, err)
			if rollbackErr != nil {
				startErr = errors.Join(startErr, rollbackErr)
			}
			// Rollback done: the Runtime is defunct (D3), so release the
			// signal and the Bus; Start refuses to run again.
			r.lifecycle.stopped = true
			r.cleanup()
			// Recording the transition is the Runtime's business; reporting the
			// error is the caller's (App logs it too, with more context).
			r.log.Error("runtime start failed, rolled back", "err", startErr)
			return startErr
		}
		started = append(started, id)
	}

	r.lifecycle.started = true
	r.log.Info("runtime started")
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
// an already-closed channel, so callers must serialize (the App does, under
// its mutex).
func (r *Runtime) Stop() error {
	if r.lifecycle.stopped {
		return nil
	}

	r.log.Info("runtime stopping")
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
	r.log.Info("runtime stopped")
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
