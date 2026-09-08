package fe

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	dag "github.com/n24-x/dag-go"
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
//   - Instance goroutines may call [Runtime.Instance] and [Runtime.Context]
//     concurrently: after NewRuntime the instances map is never written
//     again, so concurrent reads are race-free. (Instances reach the
//     Runtime only through the narrow [RuntimeAccess] view, which exposes
//     exactly these two methods.)
//   - Start/Stop are NOT safe for concurrent use with each other: they
//     mutate the one-shot lifecycle state. The framework serializes them by
//     construction — the Manager (Apply/Stop, mutex-guarded) or the
//     application's single main goroutine drives the lifecycle; instance
//     goroutines never hold a *Runtime and cannot reach Start/Stop.
//
// TODO(next):
// Three orthogonal channels (see issue.md, review 4):
//   - ctx: the Runtime's lifecycle signal (standard context). Instances that
//     spawn goroutines select on ctx.Done(); Stop() cancels it.
//   - Bus: state-change notifications (one-way, no return value).
//   - (a future info container: identity/deps/logger handed to instances).
type Runtime struct {
	// ctx is the Runtime's lifecycle signal: created in NewRuntime, canceled
	// by Stop() and on failed construction. Exposed read-only via Context()
	// for instance goroutines to select on Done(); cancel never reaches them.
	//
	// TODO(next): may be removed — how instances receive the context is
	// undecided (issue.md #6, info container).
	ctx context.Context

	// cancel cancels ctx. Runtime-owned only (Stop, failed construction,
	// rolled-back Start); never exposed to instances — one instance must
	// not kill the whole Runtime.
	//
	// TODO(next): may be removed — ownership may move out of Runtime with
	// the info container (issue.md #6).
	cancel context.CancelFunc

	Bus EventBus

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
// with Stop, which cancels its context without starting anything.
func NewRuntime(mc *feconfig.MachineConfig) (*Runtime, error) {
	if err := ValidateRuntimeConfig(mc); err != nil {
		return nil, err
	}

	order, err := instanceOrder(mc)
	if err != nil {
		return nil, fmt.Errorf("fe: new runtime: resolving instance order: %w", err)
	}

	// Lifecycle context: canceled when construction fails below, or when
	// the Runtime is stopped (Stop). Instances' goroutines select on it.
	ctx, cancel := context.WithCancel(context.Background())

	r := &Runtime{
		ctx:       ctx,
		cancel:    cancel,
		cfg:       mc,
		instances: make(map[InstanceID]Instance, len(order)),
		mods:      make(map[ModuleID]bool, len(order)),
		lifecycle: lifecycle{startOrder: make([]InstanceID, 0, len(order))},
	}

	for _, spec := range order {
		id, err := uuid.Parse(spec.InstanceID)
		if err != nil {
			cancel() // construction failed: release the lifecycle ctx
			return nil, fmt.Errorf("fe: new runtime: invalid instance id %q: %w", spec.InstanceID, err)
		}

		inst, err := r.provision(*spec)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("fe: new runtime: instance %q (%s): %w", spec.InstanceID, spec.ModuleID, err)
		}
		if inst == nil {
			cancel()
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
func (r *Runtime) provision(spec feconfig.InstanceSpec) (Instance, error) {
	m, err := GetModule(ModuleID(spec.ModuleID))
	if err != nil {
		return nil, err
	}
	return m.(Provisioner).Provision(spec, r)
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
//     apps); the context cancel signals any goroutines it may have spawned
//     to wind down (there is no resource layer to release — D25).
//   - after a failed Start the Runtime is left stopped/defunct: the context
//     is canceled, and Start refuses to run again.
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
			r.cancel()
			r.lifecycle.stopped = true
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
// start order, then cancels the lifecycle context so instance goroutines
// wind down.
//
// Rules:
//   - instances that were provisioned but never started (Stop before Start,
//     or a rolled-back Start) are not stopped — they never began. There is
//     nothing to release for them either: by contract (D25) Provision is
//     side-effect-free (config parsing + dependency capture only), so all
//     resources are Start/Stop-scoped; the context cancel still signals any
//     goroutines they may have spawned.
//   - Stop is idempotent: it may be called any number of times, before or
//     after Start, and after a failed Start. The context is canceled each
//     time; canceling twice is harmless.
//   - errors from individual Stop calls are aggregated with errors.Join.
//
// Not safe for concurrent use with Start.
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
	r.cancel()
	return err
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

// Context returns the Runtime's lifecycle context, read-only. Instance
// goroutines select on Context().Done() to know when to stop. Stop and a
// rolled-back Start cancel it. Instances never receive the cancel function:
// canceling the whole Runtime is a framework-level lifecycle decision.
func (r *Runtime) Context() context.Context {
	return r.ctx
}

// Runtime satisfies RuntimeAccess: it can be passed to Provision directly.
var _ RuntimeAccess = (*Runtime)(nil)
