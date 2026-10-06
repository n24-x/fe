package directivefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/eventbus"
	"github.com/n24-x/fe/feconfig"
)

// The adapter registry is process-global and Register panics on a duplicate
// directive, so each fixture adapter below is registered exactly once for the
// whole test binary.
const (
	adaptTag = "primary"

	adaptDirective = "adapttest"
	adaptModuleID  = fe.ModuleID("adapttest.mod")

	adaptDirectDirective = "adapttestdirect"
	adaptDirectModuleID  = fe.ModuleID("adapttest.direct")
)

// adaptModule only exists to be a registry citizen: the adapt pass needs a
// fe.Module to register, and it is never instantiated by the framework here.
type adaptModule struct{}

func (adaptModule) FeModule() fe.ModuleInfo { return fe.ModuleInfo{ID: adaptModuleID} }

// adaptDirectModule is the DirectInstantiateType twin: the adapt pass
// synthesises its commands, so it is configured with no DirectiveFile input.
type adaptDirectModule struct{}

func (adaptDirectModule) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: adaptDirectModuleID}
}

// adaptAdapter is an AdaptAndInstantiateType adapter. Its Run resolves the
// tag declared in the DirectiveFile, so a tag that the adapt pass failed to
// register surfaces as an error instead of a silently different config.
type adaptAdapter struct{}

func (adaptAdapter) ModuleHandler() ModuleHandler {
	return ModuleHandler{
		Module:     adaptModule{},
		ModuleType: AdaptAndInstantiateType,
		Unmarshaler: Unmarshaler{
			ModuleDirective: adaptDirective,
			RegisterTag:     DefaultParseTagFunc,
			Run: func(cc CommandContext, cmd Command) (feconfig.InstanceSpec, error) {
				id, err := cc.InstanceIDByTag(adaptTag)
				if err != nil {
					return feconfig.InstanceSpec{}, fmt.Errorf("resolving tag %q: %w", adaptTag, err)
				}

				config, err := json.Marshal(cmd.Args)
				if err != nil {
					return feconfig.InstanceSpec{}, err
				}

				return feconfig.InstanceSpec{
					InstanceID: id.String(),
					ModuleID:   string(adaptModuleID),
					Config:     config,
				}, nil
			},
		},
	}
}

// adaptDirectAdapter is a DirectInstantiateType adapter for two instances.
// Its synthesised command carries the pre-minted instance id as its only
// argument, which is what Run reports back.
type adaptDirectAdapter struct{}

func (adaptDirectAdapter) ModuleHandler() ModuleHandler {
	return ModuleHandler{
		Module:                  adaptDirectModule{},
		ModuleType:              DirectInstantiateType,
		DirectInstantiateNumber: 2,
		Unmarshaler: Unmarshaler{
			ModuleDirective: adaptDirectDirective,
			Run: func(_ CommandContext, cmd Command) (feconfig.InstanceSpec, error) {
				if len(cmd.Args) != 1 {
					return feconfig.InstanceSpec{}, fmt.Errorf(
						"%s: expected 1 arg (the instance id), got %d",
						cmd.Directive,
						len(cmd.Args),
					)
				}

				return feconfig.InstanceSpec{
					InstanceID: cmd.Args[0],
					ModuleID:   string(adaptDirectModuleID),
				}, nil
			},
		},
	}
}

var registerAdaptFixtures = sync.OnceFunc(func() {
	Register(adaptAdapter{})
	Register(adaptDirectAdapter{})
})

// writeAdaptFile writes a DirectiveFile into a fresh temp dir and returns its
// path, so import paths and error messages are self-contained per test.
func writeAdaptFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// TestAdapt covers the whole parse → register-tags → run pipeline, which is
// the only place the three passes are ordered against each other.
func TestAdapt(t *testing.T) {
	registerAdaptFixtures()

	t.Run("registers the DirectiveFile tag before running", func(t *testing.T) {
		path := writeAdaptFile(t, "ok.conf", adaptDirective+" alpha beta {\n    tag "+adaptTag+"\n}\n")

		mc, err := Adapt(path, feconfig.Options{})
		if err != nil {
			t.Fatalf("Adapt() error = %v", err)
		}

		// Select by module rather than counting: the registry is global, so
		// other registered adapters contribute instances of their own.
		var mine []feconfig.InstanceSpec
		for _, inst := range mc.Instances {
			if inst.ModuleID == string(adaptModuleID) {
				mine = append(mine, inst)
			}
		}
		if len(mine) != 1 {
			t.Fatalf("expected 1 %s instance, got %d: %+v", adaptModuleID, len(mine), mc.Instances)
		}

		inst := mine[0]
		// The id is the one the tag pass minted: resolving the tag at all is
		// the regression this test exists for. The DirectiveFile's commands
		// used to be appended AFTER the tag pass, so InstanceIDByTag always
		// failed with ErrInstanceNotFound and no DirectiveFile tag ever worked.
		if inst.InstanceID == "" {
			t.Error("InstanceID is empty; the tag did not resolve to a minted id")
		}
		if got, want := string(inst.Config), `["alpha","beta"]`; got != want {
			t.Errorf("Config = %s, want %s", got, want)
		}

		if err := feconfig.MachineConfigValidate(&mc); err != nil {
			t.Errorf("MachineConfigValidate() = %v", err)
		}
	})

	t.Run("runs synthesised direct-instantiate commands too", func(t *testing.T) {
		path := writeAdaptFile(t, "direct.conf", adaptDirective+" {\n    tag "+adaptTag+"\n}\n")

		mc, err := Adapt(path, feconfig.Options{})
		if err != nil {
			t.Fatalf("Adapt() error = %v", err)
		}

		// The two the adapt pass synthesised for the DirectInstantiateType
		// module, alongside the one the DirectiveFile command produced.
		var direct int
		for _, inst := range mc.Instances {
			if inst.ModuleID == string(adaptDirectModuleID) {
				direct++
			}
		}
		if direct != 2 {
			t.Errorf("got %d direct-instantiate instances, want 2", direct)
		}

		// Distinct, well-formed v4 ids for the two synthesised instances.
		if err := feconfig.MachineConfigValidate(&mc); err != nil {
			t.Errorf("MachineConfigValidate() = %v", err)
		}
	})

	t.Run("carries Options into the MachineConfig", func(t *testing.T) {
		path := writeAdaptFile(t, "opts.conf", adaptDirective+" {\n    tag "+adaptTag+"\n}\n")

		want := feconfig.Options{
			Bus: eventbus.BusOptions{
				Disable:        true,
				RouterCapacity: 16,
			},
			Logger: slog.Default(),
		}

		mc, err := Adapt(path, want)
		if err != nil {
			t.Fatalf("Adapt() error = %v", err)
		}

		if mc.Options.Logger != want.Logger {
			t.Errorf("Options.Logger = %v, want %v", mc.Options.Logger, want.Logger)
		}
		if mc.Options.Bus != want.Bus {
			t.Errorf("Options.Bus = %+v, want %+v", mc.Options.Bus, want.Bus)
		}
	})

	t.Run("returns the parse error for a missing file", func(t *testing.T) {
		_, err := Adapt(filepath.Join(t.TempDir(), "absent.conf"), feconfig.Options{})
		if err == nil {
			t.Fatal("expected an error for a missing file")
		}
	})

	t.Run("rejects an unregistered directive", func(t *testing.T) {
		path := writeAdaptFile(t, "unknown.conf", "nosuchdirective alpha\n")

		_, err := Adapt(path, feconfig.Options{})
		if !errors.Is(err, ErrAdapterNotRegistered) {
			t.Fatalf("expected %v, got %v", ErrAdapterNotRegistered, err)
		}
	})
}
