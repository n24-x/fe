package fecmd

import "github.com/spf13/cobra"

func FeCmdToCobra(feCmd Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   feCmd.Name + " " + feCmd.Usage,
		Short: feCmd.Short,
		Long:  feCmd.Long,
	}

	feCmd.CobraFunc(cmd)

	return cmd
}

// CommandFuncToCobraRunE wraps a Fe [CommandFunc] for use
// in a cobra command's RunE field.
func CommandFuncToCobraRunE(f CommandFunc) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// wrap cobra flags → fe flags, then execute command
		status, err := f(Flags{cmd.Flags()}) // key point
		if status > 1 {
			cmd.SilenceErrors = true
			return &ExitError{ExitCode: status, Err: err}
		}
		return err
	}
}
