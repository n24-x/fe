package directivefile

import (
	"fmt"
	"sync"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
)

var (
	adapterRegistry = map[ModuleDirective]Adapter{}
	adaptersMu      sync.RWMutex
)

type Adapter interface {
	ModuleHandler() ModuleHandler
}

type ModuleHandler struct {
	Module      fe.Module
	ModuleType  ModuleType
	Unmarshaler Unmarshaler

	// DirectInstantiateNumber is how many instances to create for a module
	// whose ModuleType is DirectInstantiateType. Zero (the default) means one.
	DirectInstantiateNumber int
}

// Unmarshaler turns a DirectiveFile Command into a feconfig.InstanceSpec;
// the specs are collected into a feconfig.MachineConfig.
type Unmarshaler struct {
	// Composable reports whether this Module allows the DirectiveFile to be
	// extended by other Modules. It only applies to AdaptAndInstantiateType.
	//   true  — keep unrecognised Sub Commands and hand them to
	//           RegisterTag / Run
	//   false — reject an unrecognised Sub Command
	//
	// When Composable is true, RegisterTag MUST be set: a kept Sub Command can
	// only be handled by RegisterTag, so leaving it nil silently drops it.
	// This is a promise the Module author makes to the framework; nothing
	// enforces it, and this comment is the only place it is recorded.
	Composable      bool
	ModuleDirective ModuleDirective
	SubCommandNames map[string]struct{} // TODO change to container/set when go1.28
	RegisterTag     func(CommandContext, Command)
	// Run 处理该 Command 的 Args 与 Sub Command，返回它对应的 feconfig.InstanceSpec。
	Run func(CommandContext, Command) (feconfig.InstanceSpec, error)
}

// Module Directive
// eg dns dns.forwarder
type ModuleDirective string

// ModuleType defines how a module is registered and instantiated.
type ModuleType uint8

const (
	OnlyRegisterType ModuleType = iota
	DirectInstantiateType
	AdaptAndInstantiateType
)

func Register(ap Adapter) {
	adaptersMu.Lock()
	defer adaptersMu.Unlock()

	handler := ap.ModuleHandler()

	fe.RegisterModule(handler.Module)
	if handler.ModuleType == OnlyRegisterType {
		return
	}

	moduleDirective := handler.Unmarshaler.ModuleDirective
	if moduleDirective == "" {
		panic("directivefile: module directive missing")
	}

	switch moduleDirective {
	case "tag", "import", "tmpl", "fe":
		panic(fmt.Sprintf("directivefile: module directive %q is reserved", moduleDirective))
	}

	if _, ok := adapterRegistry[moduleDirective]; ok {
		panic(fmt.Sprintf("directivefile: moduledirective already registered: %s", moduleDirective))
	}
	adapterRegistry[moduleDirective] = ap
}

func GetAdapter(md ModuleDirective) (Adapter, error) {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()

	ap, ok := adapterRegistry[md]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAdapterNotRegistered, md)
	}

	return ap, nil
}
