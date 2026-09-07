package fecmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

// DefaultRootFactory is the default command registry of the binary. The
// package-level [RegisterCommand] and [Commands] operate on it, and [Run]
// builds and executes it.
//
// The root command is not fixed by fe; customize it in one of two ways:
//
//   - Recommended: replace DefaultRootFactory with your own factory for full
//     control of the root command (before any command is registered).
//   - Or keep the default and set DefaultRootCmdName / DefaultRootCmdShortUsage
//     before calling [Run]; they are read lazily when the root command is
//     built, so setting them in main() is safe.
var DefaultRootFactory = NewRootCmdFactory(func() *cobra.Command {
	return &cobra.Command{
		Use:           DefaultRootCmdName,
		Short:         DefaultRootCmdShort,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
})

// DefaultRootCmdName is the root command name, i.e. the name shown in
// usage and help output. The default constructor reads it lazily, when the
// root command is built, so assigning it in main() before [Run] is enough.
//
// It can also be injected at build time:
//
//	go build -ldflags "-X github.com/n24-x/fe/fecmd.DefaultRootCmdName=myproj"
var DefaultRootCmdName = ""

// DefaultRootCmdShort is the one-line description of the root command
// shown in help output.
//
// It can also be injected at build time:
//
//	go build -ldflags "-X github.com/n24-x/fe/fecmd.DefaultRootCmdShortUsage=myproj is a ..."
var DefaultRootCmdShort = ""

func RegisterCommand(cmd Command) { DefaultRootFactory.RegisterCommand(cmd) }

func Commands() map[string]Command { return DefaultRootFactory.Commands() }

func Run() {
	if err := DefaultRootFactory.Build().Execute(); err != nil {
		if exitErr, ok := errors.AsType[*ExitError](err); ok {
			os.Exit(exitErr.ExitCode)
		}
		os.Exit(1)
	}
}
