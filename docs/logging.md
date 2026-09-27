# Logging

Logging in fe has one seam: the application owns the sink, and the framework
only writes records into it.

A framework user supplies a `slog.Handler` to `New`; fe never chooses a
backend, a destination, a format or a level. That choice is what the handler
already is, so switching backends is one expression on the application's side
with no change in fe — for instance, to route fe's records through zap
(`zapslog` turns a `zapcore.Core` into a `slog.Handler`):

```go
core := zapcore.NewCore(enc, sink, level)
app, _ := fe.New(fe.Options{
	Name:        "core",
	SlogHandler: zapslog.NewHandler(core),
})
```

Attribution is derived, never configured: each layer builds its logger from the
one above it with `slog.Logger.With`, so a record carries the whole chain
(`app → runtime`). fe adds no level of its own — levels live in the handler,
where the application already controls them.

Two rules keep the seam safe:

- A missing handler must become a discarding one, never nil: a nil
  `slog.Handler` behind a non-nil `slog.Logger` panics on first use
  (`(*slog.Logger).log` dereferences the handler). Discarding is also the right
  default for a library — fe stays silent until asked to speak.
- The logger is snapshotted when a Runtime is built, so a Runtime's output never
  changes under it.

## Where this lives

| Place | Role |
|---|---|
| `logging.go` | The seam itself: `loggerFrom` (nil → `slog.DiscardHandler`) and `ensureLogger` (a logger handed along by value may legitimately be absent) |
| `fe.Options.SlogHandler` | The handler the application passes in |
| `feconfig.Options.Logger` | The framework's channel for handing a derived logger to a Runtime; not config, carries `json:"-"` |
| `Runtime.log` | Snapshotted at construction, so a Runtime's output never changes under it |