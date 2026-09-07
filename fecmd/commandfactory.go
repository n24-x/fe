package fecmd

import (
	"maps"
	"sync"

	"github.com/spf13/cobra"
)

type RootCmdFactory struct {
	constructor func() *cobra.Command
	options     []func(*cobra.Command)

	// stores all registered commands
	commands map[string]Command
	mu       sync.RWMutex
}

func NewRootCmdFactory(fn func() *cobra.Command) *RootCmdFactory {
	return &RootCmdFactory{
		constructor: fn,
		commands:    make(map[string]Command),
	}
}

func (f *RootCmdFactory) Apply(fn func(cmd *cobra.Command)) {
	f.options = append(f.options, fn)
}

func (f *RootCmdFactory) Build() *cobra.Command {
	o := f.constructor()
	for _, v := range f.options {
		v(o)
	}
	return o
}

// RegisterCommand registers the command cmd.
// cmd.Name must be unique and conform to the
// following format:
//
//   - lowercase
//   - alphanumeric and hyphen characters only
//   - cannot start or end with a hyphen
//   - hyphen cannot be adjacent to another hyphen
//
// This function panics if the name is already registered,
// if the name does not meet the described format, or if
// any of the fields are missing from cmd.
func (factory *RootCmdFactory) RegisterCommand(cmd Command) {
	factory.mu.Lock()
	defer factory.mu.Unlock()

	if cmd.Name == "" {
		panic("command name is required")
	}
	if cmd.CobraFunc == nil {
		panic("command function missing")
	}
	if cmd.Short == "" {
		panic("command short string is required")
	}
	if _, exists := factory.commands[cmd.Name]; exists {
		panic("command already registered: " + cmd.Name)
	}
	if !commandNameRegex.MatchString(cmd.Name) {
		panic("invalid command name")
	}
	factory.Apply(func(rootCmd *cobra.Command) {
		rootCmd.AddCommand(FeCmdToCobra(cmd))
	})
	factory.commands[cmd.Name] = cmd
}

func (f *RootCmdFactory) Commands() map[string]Command {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return maps.Clone(f.commands)
}
