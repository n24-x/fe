package fecmd

import "fmt"

// exitError carries the exit code from CommandFunc to Main()
type ExitError struct {
	ExitCode int
	Err      error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exiting with code %d", e.ExitCode)
	}
	return e.Err.Error()
}
