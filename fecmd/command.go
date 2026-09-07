package fecmd

import (
	"regexp"

	"github.com/spf13/cobra"
)

// Command represents a subcommand.
// Name, CobraFunc, and Short are required.
type Command struct {
	// The name of the subcommand. Must conform to the
	// format defined by the [RegisterCommand()]
	Name string

	// Usage is a brief message describing the syntax of
	// the subcommand's flags and args. Use [] to indicate
	// optional parameters and <> to enclose literal values
	// intended to be replaced by the user. Do not prefix
	// the string with "fe" or the name of the command
	// since these will be prepended for you; only include
	// the actual parameters for this command.
	Usage string

	// Short is a one-line message explaining what the
	// command does. Should not end with punctuation.
	// Required.
	Short string

	// Long is the full help text shown to the user.
	// Will be trimmed of whitespace on both ends before
	// being printed.
	Long string

	// CobraFunc configures the command using Cobra APIs.
	CobraFunc func(*cobra.Command)
}

// CommandFunc is a command's function.
// It runs the command and returns the proper
// exit code along with any error that occurred.
type CommandFunc func(Flags) (int, error)

var commandNameRegex = regexp.MustCompile(`^[a-z0-9]$|^([a-z0-9]+-?[a-z0-9]*)+[a-z0-9]$`)
