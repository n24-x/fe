# fe

`fe` is a Mod + Instance + EventBus + Runtime + Command framework for Go,
modeled on Caddy's module architecture.

## Model

- **Module** — a stateless, registered module type (identity only, via
  `FeModule() ModuleInfo`). Modules register themselves through
  `RegisterModule` as an import side effect; the registry is process-wide.
  Modules that produce instances additionally implement `Provisioner`.
- **Instance** — one runtime individual per MachineConfig entry, produced by
  `Provisioner.Provision(spec, rt)`. A pure lifecycle: `Start() error` /
  `Stop() error`. Config parsing and semantic checks are the module's job;
  Provision is side-effect-free (config + dependency capture only).
- **MachineConfig** (`feconfig`) — JSON envelope:
  `instances: [{id (uuid4), mod_id, config, deps}]`. Syntax validation
  (`MachineConfigValidate`) lives in feconfig; registry-level semantic
  validation (`fe.ValidateRuntimeConfig`) lives in fe.
- **Runtime** — per-config container: instances keyed by id, the topological
  creation order, a per-Runtime EventBus, and a lifecycle context
  (`Runtime.Context()`). `Start()` runs instances deps-first (rollback on
  failure); `Stop()` runs them in reverse and cancels the context. Both are
  one-shot and idempotent as documented in the code.
- **Manager** — process-level owner of the active Runtime; `Apply` is the
  future reload entry point (build new tree → swap → stop old).

## Try it

`internal/testing/demo` drives a MachineConfig through validation →
instantiation → start/stop:

    go run ./internal/testing/demo ./internal/testing/demo/machine-config.json