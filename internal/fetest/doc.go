// Package fetest holds the framework author's own test scaffolding: the parts
// of it that are specific to fe.
//
// It is the home of the helpers that every test in the repository can share,
// and it is deliberately free of any dependency on fe itself — see "No
// dependency on fe" below.
//
// # What belongs here
//
// Only what a general assertion library cannot give. Assertions — equality,
// error matching, ordering, "must not panic" — are testify's job
// (github.com/stretchr/testify), and a helper that merely wraps one belongs
// there rather than here. What testify cannot know is fe's own shapes: an
// Instance that records its Start/Stop and can be made to fail, a module
// registered under a unique id, a MachineConfig built for a specific case.
// Those are what this package, and its mocks subpackage, are for.
//
// The dividing line, stated once: if testify can express it, it does not
// belong here.
//
// # No dependency on fe
//
// Nothing under fetest imports fe. That is the point of the package, not an
// accident, and it has one concrete consequence: a test file in package fe can
// import fetest. A package that imports fe cannot be imported from an internal
// test of fe — the go command rejects it with "import cycle not allowed in
// test" — so a fetest that named fe types would be usable only by external
// tests, and the helpers would end up duplicated on the inside anyway.
//
// The cost is that fakes of fe's own interfaces cannot live here. A fake of
// RuntimeAccess, for instance, has to spell out fe.Instance and
// eventbus.Client in its method signatures, so it must import fe. Those fakes
// stay with the tests that need them.
//
// # Its counterpart
//
// internal/demo is the framework author playing the other two roles — driving
// an App like a framework user, writing modules like a module author — to find
// out whether the framework is usable from the outside. Nothing in it is test
// scaffolding, it is not shared by tests, and it is planned to be removed once
// v1.0.0 reaches main. fetest is written from the inside, for the inside.
//
// It is internal, so third-party module authors cannot reach it. That is
// intentional for now: the framework promises that RuntimeAccess can be faked
// (it is an interface with three methods), not that it ships a fake.
package fetest
