package fecmd
package fecmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newTestFactory returns a RootCmdFactory whose root command has the given
// Use string.
func newTestFactory(use string) *RootCmdFactory {
	return NewRootCmdFactory(func() *cobra.Command {
		return &cobra.Command{Use: use}
	})
}

func TestFeCmdToCobra(t *testing.T) {
	feCmd := Command{
		Name:  "serve",
		Usage: "[dir]",
		Short: "Serve files",
		Long:  "The full help text.",
		CobraFunc: func(cmd *cobra.Command) {
			cmd.Flags().IntP("port", "p", 8080, "port to listen on")
		},
	}

	cmd := FeCmdToCobra(feCmd)
	if cmd.Use != "serve [dir]" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "serve [dir]")
	}
	if cmd.Short != "Serve files" {
		t.Fatalf("Short = %q", cmd.Short)
	}
	if cmd.Long != "The full help text." {
		t.Fatalf("Long = %q", cmd.Long)
	}
	if cmd.Flags().Lookup("port") == nil {
		t.Fatal("CobraFunc was not applied: no port flag registered")
	}
}

func TestRootCmdFactoryRegisterAndBuild(t *testing.T) {
	f := newTestFactory("root")

	serve := Command{Name: "serve", Short: "Serve", CobraFunc: func(*cobra.Command) {}}
	run := Command{Name: "run-check", Short: "Run a check", CobraFunc: func(*cobra.Command) {}}
	f.RegisterCommand(serve)
	f.RegisterCommand(run)

	got := f.Commands()
	if len(got) != 2 {
		t.Fatalf("Commands() has %d entries, want 2", len(got))
	}
	// the returned map is a clone: mutating it must not affect the factory
	delete(got, "serve")
	if len(f.Commands()) != 2 {
		t.Fatal("Commands() must return a copy, not the internal map")
	}

	root := f.Build()
	if root.Name() != "root" {
		t.Fatalf("root.Name() = %q", root.Name())
	}
	names := make([]string, 0, len(root.Commands()))
	for _, sub := range root.Commands() {
		names = append(names, sub.Name())
	}
	if len(names) != 2 {
		t.Fatalf("built root has %d subcommands (%v), want 2", len(names), names)
	}
}

func TestRegisterCommandPanics(t *testing.T) {
	noop := func(*cobra.Command) {}
	tests := []struct {
		name    string
		cmd     Command
		wantMsg string
	}{
		{"empty name", Command{Short: "x", CobraFunc: noop}, "name is required"},
		{"missing CobraFunc", Command{Name: "serve", Short: "x"}, "function missing"},
		{"missing Short", Command{Name: "serve", CobraFunc: noop}, "short string"},
		{"uppercase", Command{Name: "Serve", Short: "x", CobraFunc: noop}, "invalid command name"},
		{"leading hyphen", Command{Name: "-serve", Short: "x", CobraFunc: noop}, "invalid command name"},
		{"trailing hyphen", Command{Name: "serve-", Short: "x", CobraFunc: noop}, "invalid command name"},
		{"double hyphen", Command{Name: "a--b", Short: "x", CobraFunc: noop}, "invalid command name"},
		{"underscore", Command{Name: "a_b", Short: "x", CobraFunc: noop}, "invalid command name"},
		{"space", Command{Name: "a b", Short: "x", CobraFunc: noop}, "invalid command name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFactory("root")
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("RegisterCommand(%q): expected panic", tt.cmd.Name)
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, tt.wantMsg) {
					t.Fatalf("panic = %v, want contains %q", r, tt.wantMsg)
				}
			}()
			f.RegisterCommand(tt.cmd)
		})
	}
}

func TestRegisterCommandDuplicatePanics(t *testing.T) {
	f := newTestFactory("root")
	noop := func(*cobra.Command) {}
	f.RegisterCommand(Command{Name: "serve", Short: "x", CobraFunc: noop})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("RegisterCommand duplicate: expected panic")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "already registered") {
			t.Fatalf("panic = %v, want contains %q", r, "already registered")
		}
	}()
	f.RegisterCommand(Command{Name: "serve", Short: "y", CobraFunc: noop})
}

func TestCommandNameRegex(t *testing.T) {
	valid := []string{"a", "r2", "serve", "file-server", "abc-123-def"}
	invalid := []string{"A", "-a", "a-", "a--b", "a_b", "a b", ""}

	for _, name := range valid {
		if !commandNameRegex.MatchString(name) {
			t.Errorf("commandNameRegex should accept %q", name)
		}
	}
	for _, name := range invalid {
		if commandNameRegex.MatchString(name) {
			t.Errorf("commandNameRegex should reject %q", name)
		}
	}
}

func TestCommandFuncToCobraRunE(t *testing.T) {
	boom := errors.New("boom")

	// status > 1 → wrapped as *ExitError, cobra silenced
	cmd := &cobra.Command{Use: "t"}
	runE := CommandFuncToCobraRunE(func(Flags) (int, error) { return 3, boom })
	cmd.SilenceErrors = false
	err := runE(cmd, nil)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runE(3, err): want *ExitError, got %v", err)
	}
	if exitErr.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", exitErr.ExitCode)
	}
	if !errors.Is(exitErr.Err, boom) {
		t.Fatalf("ExitError.Err = %v, want boom", exitErr.Err)
	}
	if !cmd.SilenceErrors {
		t.Fatal("SilenceErrors must be set when exit status > 1")
	}

	// status 1 → plain error, no ExitError, cobra not silenced
	cmd2 := &cobra.Command{Use: "t"}
	runE2 := CommandFuncToCobraRunE(func(Flags) (int, error) { return 1, boom })
	err = runE2(cmd2, nil)
	if err == nil {
		t.Fatal("runE(1, err): expected the error to pass through")
	}
	if errors.As(err, new(*ExitError)) {
		t.Fatalf("runE(1, err): must not wrap in ExitError, got %v", err)
	}
	if cmd2.SilenceErrors {
		t.Fatal("SilenceErrors must stay false when exit status is 1")
	}

	// status 0, nil error → nil
	runE3 := CommandFuncToCobraRunE(func(Flags) (int, error) { return 0, nil })
	if err := runE3(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runE(0, nil): unexpected error %v", err)
	}
}

func TestExitErrorError(t *testing.T) {
	if msg := (&ExitError{ExitCode: 2}).Error(); msg != "exiting with code 2" {
		t.Fatalf("Error() without Err = %q", msg)
	}
	if msg := (&ExitError{ExitCode: 2, Err: errors.New("boom")}).Error(); msg != "boom" {
		t.Fatalf("Error() with Err = %q", msg)
	}
}
