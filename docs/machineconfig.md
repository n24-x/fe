# Machine Config

Machine Config (MC) is the configuration the framework consumes: a plain
in-process value listing the Module Instances to create and run, plus the
framework's own facility options.

It is a **machine-facing format, not a human-facing one**. MC exists for `fe` to
use, not for people to hand-write: the field names and JSON tags in
`feconfig.MachineConfig` define the shape the framework consumes. Use whatever
human-friendly format suits your application — YAML, TOML, a directive file,
generated code — and convert it to a `MachineConfig` before starting. The only
requirement is that what is passed to `fe` is a `MachineConfig`.

```go
type MachineConfig struct {
	Options   Options        `json:"options"`
	Instances []InstanceSpec `json:"instances"`
}
```

Each instance is expected by its `id` (a v4 UUID, unique in the config); `deps`
on other instances drive provision/start/stop order.

| Field | Required | Meaning |
|---|---|---|
| `options` | no | Config for the framework's own facilities, not a shared area for instances. |
| `options.bus.disable` | no | Disables the EventBus. |
| `options.bus.router_capacity` | no | Router input channel capacity (default `128`). |
| `options.logger` | — | Injected by the framework at `App.Start`; `json:"-"`, never written in a config file. |
| `instances[].id` | yes | v4 UUID, unique in the config. |
| `instances[].mod_id` | yes | Registered module that produces the instance. |
| `instances[].config` | no | The module's own config; the framework hands it over untouched (`{}` if empty). |
| `instances[].deps` | no | Ids of instances this one depends on (`[]` if none). |

Need shared state across instances? Have an instance **depend on a config-source
instance** via `deps` — `options` is not a global dumping ground.

```json
{
  "options": { "bus": { "router_capacity": 8 } },
  "instances": [
    { "id": "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e", "mod_id": "logger",
      "config": { "level": "INFO" }, "deps": [] },
    { "id": "3f2504e0-4f89-41d3-9a0c-0305e82c3301", "mod_id": "dns",
      "config": { "logger_inst": "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e" },
      "deps": ["9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"] }
  ]
}
```