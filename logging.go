package fe

import "log/slog"

// Logging in fe has one seam: the application owns the sink, and the framework
// only writes records into it.
//
// A framework user supplies an [slog.Handler] to [New]; fe never chooses a
// backend, a destination, a format or a level. That choice is what the handler
// already is, so switching backends is one expression on the application's
// side with no change here — for instance, to route fe's records through zap
// (zapslog turns a zapcore.Core into an slog.Handler):
//
//	core := zapcore.NewCore(enc, sink, level)
//	app, _ := fe.New(fe.Options{
//		Name:        "core",
//		SlogHandler: zapslog.NewHandler(core),
//	})
//
// Attribution is derived, never configured: each layer builds its logger from
// the one above it with [slog.Logger.With], so a record carries the whole
// chain (app → runtime). fe adds no level of its own — levels live in the
// handler, where the application already controls them.
//
// Two rules keep the seam safe:
//
//   - A missing handler must become a discarding one, never nil: a nil
//     [slog.Handler] behind a non-nil [slog.Logger] panics on first use
//     ((*slog.Logger).log dereferences the handler). Discarding is also the
//     right default for a library — fe stays silent until asked to speak.
//   - The logger is snapshotted when a Runtime is built, so a Runtime's
//     output never changes under it.

// loggerFrom returns a logger writing to h, or one that discards everything
// when h is nil. See the note on nil handlers above.
func loggerFrom(h slog.Handler) *slog.Logger {
	if h == nil {
		h = slog.DiscardHandler
	}
	return slog.New(h)
}

// orDiscard returns l unchanged, or a discarding logger when l is nil. It
// guards the go-between case: a logger handed along by value — a zero-value
// App's, or mc.Options.Logger — can legitimately be absent, and callers must
// not have to check.
func orDiscard(l *slog.Logger) *slog.Logger {
	if l == nil {
		return loggerFrom(nil)
	}
	return l
}
