package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
)

func init() {
	fe.RegisterModule(Module{})
}

type Module struct{}

var (
	_ fe.Module      = Module{}
	_ fe.Provisioner = Module{}
)

func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "logger"}
}

type config struct {
	Level  string `json:"level"`
	Output string `json:"output"`
}

func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("logger: decoding config: %w", err)
		}
	}
	if cfg.Level == "" {
		cfg.Level = "info" // default
	}
	return &Instance{level: cfg.Level, output: cfg.Output}, nil
}

type LogProvider interface {
	Logger() *slog.Logger
	Named(name string) *slog.Logger
}

type Instance struct {
	level  string
	output string
	logger *slog.Logger
	closer io.Closer
}

var (
	_ fe.Instance = (*Instance)(nil)
	_ LogProvider = (*Instance)(nil)
)

var discard = slog.New(slog.DiscardHandler)

func (i *Instance) Start() error {
	level, err := parseLevel(i.level)
	if err != nil {
		return err
	}

	var w io.Writer = os.Stdout
	if i.output != "" {
		f, err := os.OpenFile(i.output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("logger: opening output %q: %w", i.output, err)
		}
		w = f
		i.closer = f
	}

	i.logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})).
		With("application", "mydemo")
	return nil
}
func (i *Instance) Stop() error {
	if i.closer == nil {
		return nil
	}
	err := i.closer.Close()
	i.closer = nil
	return err
}

func (i *Instance) Logger() *slog.Logger {
	if i.logger == nil {
		return discard // before Start
	}
	return i.logger
}

func (i *Instance) Named(name string) *slog.Logger {
	return i.Logger().With("instance", name)
}

func (i *Instance) Level() string  { return i.level }
func (i *Instance) Output() string { return i.output }

func parseLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("logger: %w", err)
	}
	return level, nil
}
