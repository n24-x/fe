package fe

// EventBus is the per-Runtime state-change notification channel: one-way,
// no return value, no ack, no timeout (issue.md review 1 — "not a Q&A").
// Every Runtime owns its own Bus, rebuilt along with the Runtime on reload.
//
// Scaffold only: the publish/subscribe implementation is not written yet
// (issue.md §7); nothing references it today.
type EventBus struct{}
