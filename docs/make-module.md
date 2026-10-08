# Module Development

A Module is a **registered type**. There are two kinds: a module that only
registers itself, and one that also produces instances (see
[Module Instance](module-instance.md)).

## Register

Every module implements `Module` and registers itself in `init`, so an import
side effect is enough to make it known:

```go
func init() { fe.RegisterModule(Module{}) }

type Module struct{}

var _ fe.Module = Module{}

func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "logger"}
}
```

- `FeModule` **must not have side effects**; it is called by the registry.
- `ID` is a namespaced, dot-separated name (`dns.forwarder`), and must be
  unique. `RegisterModule` panics on a nil module, a missing ID, or a
  duplicate — a misconfiguration fails at startup, not at runtime.
- The registry is process-global: register once, and `Modules()` lists them.

To *produce* instances, a module also implements `Provisioner` — see
[Module Instance](module-instance.md).