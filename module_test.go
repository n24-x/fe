package fe

import (
	"strings"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// fakeMod is a Module implementation WITHOUT Provisioner (an instance-less
// module, for registry tests).
type fakeMod struct {
	info ModuleInfo
}

func (m fakeMod) FeModule() ModuleInfo { return m.info }

// fakeProvMod is a Module WITH Provisioner (an instance-producing module).
type fakeProvMod struct {
	info ModuleInfo
	fn   func(spec feconfig.InstanceSpec, rt *Runtime) (Instance, error)
}

func (m fakeProvMod) FeModule() ModuleInfo { return m.info }

func (m fakeProvMod) Provision(spec feconfig.InstanceSpec, rt *Runtime) (Instance, error) {
	if m.fn != nil {
		return m.fn(spec, rt)
	}
	return &fakeInstance{}, nil
}

// fakeInstance is a minimal Instance implementation for tests.
type fakeInstance struct{}

func (*fakeInstance) Start() error { return nil }
func (*fakeInstance) Stop() error  { return nil }

// mod returns a registerable instance-less fake Module with the given ID.
func mod(id ModuleID) fakeMod {
	return fakeMod{info: ModuleInfo{ID: id}}
}

// provMod returns a registerable instance-producing fake Module with the
// given ID and optional Provision implementation.
func provMod(id ModuleID, fn func(spec feconfig.InstanceSpec, rt *Runtime) (Instance, error)) fakeProvMod {
	return fakeProvMod{info: ModuleInfo{ID: id}, fn: fn}
}

func TestModuleInfo_String(t *testing.T) {
	tests := []struct {
		name string
		mi   ModuleInfo
		want string
	}{
		{"normal ID", ModuleInfo{ID: "a.b.c"}, "a.b.c"},
		{"single segment", ModuleInfo{ID: "foo"}, "foo"},
		{"empty ID", ModuleInfo{ID: ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mi.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModuleID_Namespace(t *testing.T) {
	tests := []struct {
		name string
		id   ModuleID
		want string
	}{
		{"no namespace", "endpoint", ""},
		{"one level", "endpoint.socks", "endpoint"},
		{"multi level", "logging.encoders.json", "logging.encoders"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Namespace(); got != tt.want {
				t.Fatalf("Namespace() = %q, want %q (id=%q)", got, tt.want, tt.id)
			}
		})
	}
}

func TestModuleID_Name(t *testing.T) {
	tests := []struct {
		name string
		id   ModuleID
		want string
	}{
		{"no namespace", "endpoint", "endpoint"},
		{"one level", "endpoint.socks", "socks"},
		{"multi level", "logging.encoders.json", "json"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q (id=%q)", got, tt.want, tt.id)
			}
		})
	}
}

func TestRegisterAndGetModule(t *testing.T) {
	const id = ModuleID("fe.test.registry.ok")
	RegisterModule(mod(id))

	m, err := GetModule(id)
	if err != nil {
		t.Fatalf("GetModule(%q): unexpected error: %v", id, err)
	}
	if m.FeModule().ID != id {
		t.Fatalf("GetModule(%q).FeModule().ID = %q", id, m.FeModule().ID)
	}
}

func TestGetModuleNotRegistered(t *testing.T) {
	const id = ModuleID("fe.test.registry.does.not.exist")
	if _, err := GetModule(id); err == nil {
		t.Fatalf("GetModule(%q): expected error for unregistered module", id)
	} else if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("GetModule(%q) error = %v, want contains %q", id, err, "not registered")
	}
}

func TestRegisterModulePanics(t *testing.T) {
	tests := []struct {
		name     string
		instance Module
		wantMsg  string
	}{
		{
			name:     "empty module ID",
			instance: fakeMod{info: ModuleInfo{}},
			wantMsg:  "module ID missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("RegisterModule(%s): expected panic, got none", tt.name)
				}
				msg, ok := r.(string)
				if !ok || !strings.Contains(msg, tt.wantMsg) {
					t.Fatalf("panic message = %v, want contains %q", r, tt.wantMsg)
				}
			}()
			RegisterModule(tt.instance)
		})
	}
}

func TestRegisterModuleDuplicate(t *testing.T) {
	const id = ModuleID("fe.test.registry.dup")
	RegisterModule(mod(id))

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("RegisterModule duplicate: expected panic, got none")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "already registered") {
			t.Fatalf("panic message = %v, want contains %q", r, "already registered")
		}
	}()
	RegisterModule(mod(id)) // second registration of the same ID must panic
}
