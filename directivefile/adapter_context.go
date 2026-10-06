package directivefile

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"uuid"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
)

type Context struct {
	context context.Context

	mu               sync.RWMutex
	instanceRegistry instanceRegistry
}

// NewContext creates the Context for one conversion run.
func NewContext(ctx context.Context) *Context {
	return &Context{
		context: ctx,
		instanceRegistry: instanceRegistry{
			implicit: make(map[fe.ModuleID][]fe.InstanceID),
			explicit: make(map[string]fe.InstanceID),
			all:      make(map[fe.InstanceID]struct{}),
		},
	}
}

type instanceRegistry struct {
	// Instances not described by the DirectiveFile: those whose ModuleType is
	// DirectInstantiateType.
	// Allow multiple instances; other instances reference them as needed.
	implicit map[fe.ModuleID][]fe.InstanceID

	// DirectiveFile instances indexed by tag.
	// A tag identifies an Instance and is resolved to its InstanceID.
	explicit map[string]fe.InstanceID

	// All instances id
	all map[fe.InstanceID]struct{}
}

// CommandContext is created fresh for every Command — never shared, never
// mutated by a parent — so it is passed by value.
type CommandContext struct {
	Context *Context

	ParentNameSpace  string
	ParentInstanceID fe.InstanceID
}

func (cc CommandContext) Child(id fe.InstanceID, directive string) CommandContext {
	if cc.ParentNameSpace == "" {
		return CommandContext{
			Context:          cc.Context,
			ParentInstanceID: id,
			ParentNameSpace:  directive,
		}
	}
	return CommandContext{
		Context:          cc.Context,
		ParentInstanceID: id,
		ParentNameSpace:  cc.ParentNameSpace + "." + directive,
	}
}

func (cc CommandContext) ImplicitInstanceIDs(modID fe.ModuleID) ([]fe.InstanceID, error) {
	return cc.Context.implicitInstanceIDs(modID)
}

func (cc CommandContext) InstanceIDByTag(tag string) (fe.InstanceID, error) {
	return cc.Context.instanceIDByTag(tag)
}

// NewInstanceID returns an id that is not yet in all, and records it.
// Caller MUST hold ctx.mu.
func (ctx *Context) newInstanceID() fe.InstanceID {
	id := fe.InstanceID(uuid.NewV4())
	for {
		if _, taken := ctx.instanceRegistry.all[id]; !taken {
			break
		}
		id = fe.InstanceID(uuid.NewV4())
	}
	ctx.instanceRegistry.all[id] = struct{}{}
	return id
}

// RegisterDirectInstantiateModule creates one Command per instance of every
// DirectInstantiateType module, and records their ids in implicit.
func (ctx *Context) registerDirectInstantiateModule() []Command {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	directives := make([]ModuleDirective, 0, len(adapterRegistry))
	for directive := range adapterRegistry {
		directives = append(directives, directive)
	}
	slices.Sort(directives)

	cmds := make([]Command, 0, len(directives))

	for _, directive := range directives {
		adapter := adapterRegistry[directive]
		handler := adapter.ModuleHandler()

		if handler.ModuleType != DirectInstantiateType {
			continue
		}

		number := handler.DirectInstantiateNumber
		if number < 0 {
			panic(fmt.Sprintf(
				"directivefile: module %q has invalid direct instantiate number %d",
				directive,
				number,
			))
		}
		if number <= 0 {
			number = 1
		}

		moduleID := handler.Module.FeModule().ID
		ids := ctx.instanceRegistry.implicit[moduleID]
		if len(ids) > 0 {
			panic(fmt.Sprintf(
				"directivefile: implicit instances already registered for module %q",
				moduleID,
			))
		}

		for range number {
			id := ctx.newInstanceID()
			ctx.instanceRegistry.implicit[moduleID] =
				append(ctx.instanceRegistry.implicit[moduleID], id)

			cmds = append(cmds, Command{
				Directive: string(directive),
				Args:      []string{id.String()},
			})
		}
	}

	return cmds
}

func (ctx *Context) implicitInstanceIDs(id fe.ModuleID) ([]fe.InstanceID, error) {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	ids, ok := ctx.instanceRegistry.implicit[id]
	if !ok {
		return []fe.InstanceID{}, fmt.Errorf("%w: %q", ErrNoImplicitInstance, id)
	}

	return ids, nil
}

func (ctx *Context) instanceIDByTag(tag string) (fe.InstanceID, error) {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	id, ok := ctx.instanceRegistry.explicit[tag]
	if !ok {
		return fe.InstanceID{}, fmt.Errorf("%w: %q", ErrInstanceNotFound, tag)
	}

	return id, nil
}

func (ctx *Context) registerTags(cmds []Command) error {
	for _, cmd := range cmds {
		if err := ctx.registerTag(cmd, CommandContext{
			Context: ctx,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) registerTag(cmd Command, cmdCtx CommandContext) error {
	adapter, err := GetAdapter(ModuleDirective(cmd.Directive))
	if err != nil {
		return err
	}

	handler := adapter.ModuleHandler()
	if handler.Unmarshaler.RegisterTag != nil {
		handler.Unmarshaler.RegisterTag(cmdCtx, cmd)
	}

	return nil
}

func (ctx *Context) run(mc *feconfig.MachineConfig, cmds []Command) error {
	for _, cmd := range cmds {
		if err := ctx.runCommand(mc, cmd, CommandContext{
			Context: ctx,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Context) runCommand(
	mc *feconfig.MachineConfig,
	cmd Command,
	cmdCtx CommandContext,
) error {
	adapter, err := GetAdapter(ModuleDirective(cmd.Directive))
	if err != nil {
		return err
	}

	handler := adapter.ModuleHandler()
	unmarshaler := handler.Unmarshaler

	if unmarshaler.Run == nil {
		return fmt.Errorf(
			"%s: module %q has no Run function",
			cmd.Source(),
			cmd.Directive,
		)
	}

	spec, err := unmarshaler.Run(cmdCtx, cmd)
	if err != nil {
		return err
	}

	mc.Instances = append(mc.Instances, spec)

	return nil
}
