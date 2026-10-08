# App & Runtime

App is the application's entry point and its handle on the framework: it owns
exactly one active Runtime. A Runtime is one running world, built from one
[MachineConfig](machineconfig.md); it owns the instances and drives
their lifecycle.

```
App.Start(mc) ──▶ NewRuntime(mc) ──▶ Runtime.Start()   instances start, dependencies first
App.Stop()    ──▶ Runtime.Stop()   instances stop, reverse order
```

## App

```go
app, err := fe.New(fe.Options{Name: "demo", SlogHandler: h})
err = app.Start(mc) // build + start a Runtime, replacing the active one
err = app.Stop()    // stop the App and its Runtime
```

- `New` never fails today; the error is reserved for future checks.
- `Start` builds and starts a Runtime, then installs it and stops the one it
  replaced — so it doubles as **reload**. On failure the active Runtime is
  unchanged.
- `Stop` is final: a stopped App cannot start again (a later `Start` returns
  `ErrAppStopped`). With no active Runtime it is a no-op, so it is safe to call
  unconditionally on the shutdown path.
- fe does not recover panics: a panic in `Provision`, `Instance.Start` or
  `Instance.Stop` unwinds through the framework, which releases what it owns on
  the way out, and carries on to the caller.

## Runtime

One Runtime = one MachineConfig. `NewRuntime` validates, orders the instances
(dependencies first) and provisions each; the returned Runtime is **created but
idle** until `Start`.

- `Start` is one-shot: it starts every instance in dependency order. If one
  fails, the already-started instances stay up — `Stop` is what takes them down.
- `Stop` is idempotent: it stops the started instances in reverse order, then
  releases the Runtime's lifecycle signal and Bus. Per-instance errors are
  joined with `errors.Join`.
- Instances work against `RuntimeAccess` — a sealed view exposing `Instance(id)`,
  `Done()` and `BusClient(name)` — so they cannot reach `*Runtime` or drive its
  lifecycle.

Lifecycle: `created → started → stopped`, or `created → stopped`.

`Start` and `Stop` are not safe to run concurrently with each other; the
framework serializes them. Instance goroutines may use `RuntimeAccess`
concurrently.

## The four nouns

| | What it is | Lifecycle |
|---|---|---|
| App | the entry point; owns the active Runtime | `Start` / `Stop` |
| Runtime | one world, built from one MachineConfig | `Start` / `Stop` |
| Module | a registered type (see `Module`) | registered at init |
| Instance | one running instance of a Module | `Start` / `Stop` |

App is to Runtime what Module is to Instance.

