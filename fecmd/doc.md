# README

# Example

```go
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/n24-x/fe/fecmd"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// 1. Build a cmd factory
	factory := fecmd.NewRootCmdFactory(func() *cobra.Command {
		return &cobra.Command{
			Use:           "fe",
			Short:         "Example CLI built on the fe framework",
			SilenceUsage:  true,  // don't spam usage text when a command fails
			SilenceErrors: false, // let cobra print "Error: ..."
		}
	})

	// 2. Register subcommands.
	factory.RegisterCommand(fecmd.Command{
		Name:  "serve",
		Short: "Start the server",
		Usage: "[--addr <host:port>]",
		CobraFunc: func(cmd *cobra.Command) {
			cmd.Flags().StringP("addr", "a", ":8080", "address to listen on")
			// Registered as a string so values like "1d" survive pflag parsing;
			// fecmd.Flags.Duration reads it back through fe/common/timex,
			// which understands the "d" (day) unit.
			cmd.Flags().StringP("timeout", "t", "0", "shutdown timeout (e.g. 30s, 1h30m, 1d)")
			// CommandFuncToCobraRunE adapts a CommandFunc to cobra's RunE,
			// wrapping the cobra flag set in a fecmd.Flags for typed access.
			cmd.RunE = fecmd.CommandFuncToCobraRunE(func(fl fecmd.Flags) (int, error) {
				fmt.Printf("fe is serving on %s (timeout: %s)\n",
					fl.String("addr"), fl.Duration("timeout"))
				return 0, nil
			})
		},
	})

	factory.RegisterCommand(fecmd.Command{
		Name:  "version",
		Short: "Show version information",
		CobraFunc: func(cmd *cobra.Command) {
			cmd.RunE = fecmd.CommandFuncToCobraRunE(func(fl fecmd.Flags) (int, error) {
				fmt.Println("fe v0.1.0")
				return 0, nil
			})
		},
	})

	// 3. Build the full command tree and execute it.
	rootCmd := factory.Build()
	rootCmd.SetArgs(args)

	if err := rootCmd.Execute(); err != nil {
		// A CommandFunc returns (status, err); a status > 1 is surfaced as an
		// *ExitError so the exact exit code survives the trip to main().
		var exitErr *fecmd.ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintln(os.Stderr, exitErr)
			return exitErr.ExitCode
		}
		return 1
	}
	return 0
}
```