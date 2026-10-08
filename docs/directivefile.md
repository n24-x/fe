# Config: DirectiveFile

A DirectiveFile is **not part of the `fe` framework**. It is an official
companion format: a text format written by hand, for humans. It sits *outside*
the framework's boundary. `Adapt` turns it into the machine-facing
[MachineConfig](machineconfig.md), which is the only thing `fe` itself consumes.

```go
mc, err := directivefile.Adapt("main.conf", feconfig.Options{})
```

## Shape

A file is a sequence of commands, one per line:

```
# comment
directive arg1 arg2 arg3 ... {
    subDirective1 arg1 arg2 ...
    subDirective2 arg1 arg2 ...
}
```

- A command is a **directive**, then any number of **arguments**, then an
  optional **block**.
- A block `{ ... }` must be the **last** thing on the line, must **start on a
  new line**, and must contain **at least one** command.
- `}` starts its own line.
- Blocks nest; the depth is capped (`maxCommandDepth = 64`).
- A file must contain at least one command.

## Directives

A directive is an unquoted token: the first token of a command. It cannot be
quoted, and it is never environment-substituted.

A name is dot-separated *namespaces*: each segment is a letter followed by
letters, digits, `_` or `-` (ASCII only).

```
dns.forwarder {
    upstream 1.1.1.1
}
```

The dot is a **name character, not punctuation** — `dns.forwarder` is one
token, so an inner block may spell its namespace out (`endpoint.http.proxy`).

`tag`, `import`, `tmpl` and `fe` are reserved; a module cannot claim them.

## Values

| Form | Behavior |
|---|---|
| `bare` | Verbatim, up to the next whitespace. No escaping — `\`, `#`, `{`, `}` are ordinary characters inside it. |
| `"quoted"` | Interpreted: `\n`, `\t`, `\\`, `\"`, `\uXXXX`, `\UXXXXXXXX`. **Cannot span lines.** |
| `` `backtick` `` | Raw: no escapes at all, and **may span lines**. The way to embed a multi-line or backslash-heavy value. |

A `\` alone at the end of a line joins it to the next (line continuation); the
two lines behave as one command.

```
command {
   help `This is command full doc...
blah blah blah blah blah
blah` "the command short doc..."
}
```

A `#` at the start of a token begins a comment, to the end of the line.

## Environment variables

`{$NAME}` — the **whole token** — is replaced by the value of environment
variable `NAME` (`[A-Za-z_][A-Za-z0-9_]*`). It must be set and non-empty.

The value becomes **exactly one argument**, whatever it contains:

```
cmd {$DNS_UPSTREAM}
```

Anything that is not exactly that shape stays a literal, with no error:
`{$1}`, `{$}`, `pre{$X}post`, `{$HOME}x`. There is no default-value syntax.

## import

```
import common/endpoints.conf
import auth.conf tls.conf
```

Paths resolve relative to the **importing file's** directory. Imports may
appear anywhere and may themselves import; cycles are detected and the chain is
capped at 32 levels. An imported file must contribute at least one command.

## Templates

A template is defined at the top level of a file, and expands to its body
wherever it is called:

```
(proxy) {
    forward {args[0]} {args[1]} {args[0]}
    header {args[1]}
}

tmpl proxy alpha beta
```

The call above expands to `forward alpha beta alpha`, then `header beta`.

- Definition: `(name) { ... }` — **top level only**, no arguments, must have a
  block, unique per file.
- Call: `tmpl <name> [args...]` — may appear **anywhere**, including inside a
  block, and cannot have a block of its own.
- `{args[N]}` in the body becomes the call's Nth argument. Indices must be
  contiguous from `0` and `N < 32` (at most 32 parameters).
- A call must pass **exactly** the arguments its body uses — no more, no
  fewer.
- Templates are **file-local**; they do not cross `import`.

## tag

`tag <name>` is an optional sub-command that names the instance its command
produces, so other commands can refer to it:

```
dns.forwarder {
    tag primary
    upstream 1.1.1.1
}

endpoint.proxy.server {
    resolver primary
}
```

A `tag` may appear at most once per command. `tag` is read at its own level
only: a `tag` inside a nested sub-command belongs to that sub-command's module.

## How a directive becomes an instance

Each registered module contributes an `Adapter` declaring its directive and an
optional `Run`. `Adapt` runs in three passes:

1. **parse** — lex, parse, expand templates and imports, validate.
2. **register tags** — so every `tag` resolves to an instance id before any
   `Run` needs it.
3. **run** — each command becomes a `feconfig.InstanceSpec`, collected into the
   MachineConfig.

A module with no `Run` is `OnlyRegisterType`: registered with `fe`, but never
instantiated from a file.
