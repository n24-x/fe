package fe

import "log/slog"

// Logging in fe has one seam: the application owns the sink, and the framework
// only writes records into it. This file is that seam. The design behind it —
// why a [slog.Handler] and not a logger, why a missing one must discard rather
// than stay nil, how attribution is derived — is written up in:
//
// https://github.com/n24-x/fe/blob/main/docs/logging.md

// loggerFrom returns a logger writing to h, or a discarding logger when h is nil.
func loggerFrom(h slog.Handler) *slog.Logger {
	if h == nil {
		h = slog.DiscardHandler
	}
	return slog.New(h)
}

// ensureLogger returns l unchanged, or a discarding logger when l is nil.
// This lets callers use an optional logger without checking for nil.
func ensureLogger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return loggerFrom(nil)
	}
	return l
}
