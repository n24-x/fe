# Adding CLI Commands

`fecmd` is fe's CLI layer, built on cobra. A command is described as a
`Command` value, registered with a factory, and run as one tree.

## The default registry

A module usually registers into the package-level registry, so an import side
effect is enough to add its command:

```go
func init() {
	fecmd.RegisterCommand(fecmd.Command{
		Name:  "serve",
		Short: "Start the server",
		CobraFunc: func(cmd *cobra.Command) {
			cmd.Flags().StringP("addr", "a", ":8080", "address to listen on")
			cmd.RunE = fecmd.CommandFuncToCobraRunE(func(fl fecmd.Flags) (int, error) {
				fmt.Println("serving on", fl.String("addr"))
				return 0, nil
			})
		},
	})
}
```

Then `main` is just:

```go
func main() {
	fecmd.DefaultRootCmdName = "myapp" // optional; read when the root is built
	fecmd.Run()
}
```

## Your own root command

An application that wants control of the root replaces the default factory
*before* registering anything:

```go
factory := fecmd.NewRootCmdFactory(func() *cobra.Command {
	return &cobra.Command{Use: "myapp", SilenceUsage: true}
})
factory.RegisterCommand(fecmd.Command{ /* ... */ })

root := factory.Build()
root.SetArgs(os.Args[1:])
if err := root.Execute(); err != nil { /* handle */ }
```

`DefaultRootCmdName` / `DefaultRootCmdShort` name the default root. Both are
read lazily, so setting them in `main` works — or inject them at build time:
`go build -ldflags "-X github.com/n24-x/fe/fecmd.DefaultRootCmdName=myapp"`.

## A Command

| Field | Required | Meaning |
|---|---|---|
| `Name` | yes | the subcommand name |
| `Short` | yes | one-line description; no trailing punctuation |
| `CobraFunc` | yes | configures the flags and the run function |
| `Usage` | no | flag/arg syntax, e.g. `[--addr <host:port>]` |
| `Long` | no | full help text |

`RegisterCommand` **panics** on a missing required field, a duplicate name, or
an invalid name. A name is lowercase alphanumeric plus hyphens, and cannot
start with, end with, or double a hyphen.

## Exit codes

The run function is `func(fecmd.Flags) (int, error)`; `CommandFuncToCobraRunE`
adapts it for cobra's `RunE`:

- status `0` or `1` — returned as-is; cobra prints a non-nil error.
- status `> 1` — wrapped in `*ExitError`, carrying the exact code, so
  `main` can recover it with `errors.As` and `os.Exit(exitErr.ExitCode)`.

## Flags

`fecmd.Flags` wraps `pflag` with typed getters: `String`, `Bool`, `Int`,
`Float64`, `Duration`. Each **panics if the flag is not registered**, and
returns the zero value if the value does not parse.

`Duration` parses through `fe/common/timex`, which understands `1d`; register
such flags as *strings* so `pflag` does not reject the unit before `fecmd`
reads it back.
