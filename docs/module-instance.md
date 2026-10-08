# Module Instance

A Module is one of two kinds:

- **Registry-only** — registering is all it does (see
  [Module Development](make-module.md)), and a MachineConfig cannot name it.
- **Instantiated** — it also implements `Provisioner`, and a MachineConfig
  names it as an `InstanceSpec`.

## The Instance interface

```go
type Instance interface {
	Start() error
	Stop() error
}
```

Its lifecycle takes no parameters: identity, dependencies and the lifecycle
signal are all captured into the instance's fields during `Provision`. The
framework drives it — `Runtime.Start` starts instances in dependency order,
`Runtime.Stop` stops them in reverse.

Two rules make it safe:

- If `Start` returns an error, the instance must be left stopped and need no
  `Stop`: `Stop` is called only after a successful `Start`. A `Start` that
  panics is the same — the framework never calls `Stop` for it, so release
  what you acquired in your own `defer`.
- After `Start` succeeds, `Stop` must release everything the instance acquired.

Neither has a timeout, and either may block for as long as the module needs;
keeping them prompt is the module author's job.

## Declaring an instance

An instance exists because the MachineConfig passed to `App.Start` lists an
`InstanceSpec` for it:

```json
{ "id": "<uuid>", "mod_id": "logger", "config": { "level": "info" }, "deps": [] }
```

`mod_id` names the module; `config` is the module's own, which the framework
never decodes; `deps` are the ids of the instances this one depends on.

## Produce instances

To be instantiated, a Module also implements `Provisioner`:

```go
var _ fe.Provisioner = Module{}

func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	var cfg config // the module decodes its own spec.Config
	if err := json.Unmarshal(spec.Config, &cfg); err != nil {
		return nil, fmt.Errorf("logger: decoding config: %w", err)
	}
	return &Instance{level: cfg.Level}, nil
}
```

`Provision` **must be side-effect-free**: it only decodes `spec.Config`,
resolves dependencies, and captures them into the instance's fields. Resources
are acquired in `Instance.Start` and released in `Instance.Stop`.

## Resolving dependencies

`rt.Instance(id)` returns another instance by its config id. `NewRuntime`
provisions dependencies first, so a `Provision` may resolve its deps directly:

```go
dep, err := rt.Instance(cfg.DNSInst)
if err != nil {
	return nil, fmt.Errorf("dns.forwarder: resolving %q: %w", cfg.DNSInst, err)
}
```

The instance declares those ids in its `deps`, so the framework starts it after
them (see [MachineConfig](machineconfig.md)).