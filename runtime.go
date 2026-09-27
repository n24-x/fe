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
// Instance goroutines may call [RuntimeAccess.Instance], [RuntimeAccess.Done],
// and [RuntimeAccess.BusClient] concurrently.
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

	// bus is the Runtime's state-change notification channel: built by
	// [NewRuntime] from the config's [feconfig.Options.Bus], released by
	// cleanup(). Instances never touch it directly — they get a client of their
	// own from [RuntimeAccess.BusClient].
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
	// Instance resolves a dependency instance by its config id — the raw uuid
	// string as it appears in the machine config. An instance becomes resolvable
	// once its Provision has returned, and [NewRuntime] provisions dependencies
	// first, so an instance may call Instance from inside its own
	// [Provisioner.Provision] to resolve the dependencies it needs.
	Instance(id string) (inst Instance, err error)

	// Done returns the Runtime's lifecycle signal: closed when the Runtime goes
	// away, and never nil.
	Done() (done <-chan struct{})

	// BusClient opens a Bus client for the calling instance. name is a
	// human-readable label for debug logs and client-scoped errors; it is not
	// required to be unique. Once the Runtime has been stopped the Bus is closed,
	// so this returns [eventbus.ErrBusClosed].
	//
	// Every call returns a fresh client, owned by the caller: close it when the
	// instance is done with it. Closing is not required — the Bus closes any
	// client still open when the Runtime stops, and [eventbus.Client.Close] is
	// idempotent, so closing twice is harmless.
	BusClient(name string) (client *eventbus.Client, err error)
}

// moduleView is the sealed [RuntimeAccess] the framework hands to instances,
// so that they cannot access [*Runtime] or control its lifecycle.
type moduleView struct{ r *Runtime }

var _ RuntimeAccess = moduleView{}

func (v moduleView) Instance(id string) (Instance, error) { return v.r.instance(id) }
func (v moduleView) Done() <-chan struct{}                { return v.r.doneSignal() }
func (v moduleView) BusClient(name string) (*eventbus.Client, error) {
	return v.r.busClient(name)
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
// machine config: every instance's mod_id MUST be registered, and its module
// MUST implement [Provisioner].
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
// Construction is three steps: [ValidateRuntimeConfig], the instance creation
// order resolved from the dependency graph (dependencies first), then
// [Provisioner.Provision] for each instance in that order; the order itself is
// recorded in lifecycle.startOrder.
//
// The returned Runtime is NOT started: instances are created but idle. The
// caller starts them with [Runtime.Start], or discards the Runtime with
// [Runtime.Stop], which releases its lifecycle signal and Bus without starting
// anything.
//
// On failure NewRuntime returns no Runtime for the caller to Stop, so every
// error path below cleans up after itself before returning (see cleanup()).
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

// instanceOrder returns instances in dependency order: dependencies first.
// It returns every instance spec exactly once.
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

// provision constructs an instance from a spec by delegating to the module's
// [Provisioner.Provision]. Config parsing is the module's responsibility; the
// type assertion is safe because [ValidateRuntimeConfig] ran first.
func (r *Runtime) provision(spec feconfig.InstanceSpec) (Instance, error) {
	m, err := GetModule(ModuleID(spec.ModuleID))
	if err != nil {
		return nil, err
	}
	return m.(Provisioner).Provision(spec, moduleView{r: r})
}

// Start starts the Runtime by starting every instance in dependency order.
// It is one-shot and failure-atomic: if an instance fails to start, all
// previously started instances are stopped in reverse order; the Runtime
// becomes stopped and cannot be started again. The error is returned.
//
// Starting an already-started or already-stopped Runtime returns an error.
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
			// Rollback done: the Runtime is defunct, so release the
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

// Stop stops every started instance in reverse start order, then cleans up the
// Runtime. It is idempotent: calls after the first return nil.
//
// If Start was never completed successfully, no instances are stopped.
// Errors from individual Stop calls are aggregated with [errors.Join].
//
// Not safe for concurrent use with Start or another Stop.
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

// cleanup releases the Runtime's framework-owned resources: it closes the
// lifecycle signal and the Bus.
//
// It MUST be called exactly once per Runtime.
func (r *Runtime) cleanup() {
	close(r.done)
	r.bus.Close()
}

// instance returns the instance whose config id is id, or an error if no such
// instance exists in this Runtime.
func (r *Runtime) instance(id string) (Instance, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("fe: instance %q: invalid id: %w", id, err)
	}
	inst, ok := r.instances[InstanceID(parsed)]
	if !ok {
		return nil, fmt.Errorf("%w: %s (declared in the referencing instance's deps?)", ErrInstanceNotFound, id)
	}
	return inst, nil
}

// doneSignal returns the Runtime's lifecycle signal, receive-only.
func (r *Runtime) doneSignal() <-chan struct{} {
	return r.done
}

// busClient opens a new Bus client named name, for the calling instance.
func (r *Runtime) busClient(name string) (client *eventbus.Client, err error) {
	return r.bus.NewClient(name)
}
