package exec

import (
	"context"
	"fmt"
	"io"
)

type Cmd interface {
	Run() error
	SetEnv(...string) Cmd
	SetStdin(io.Reader) Cmd
	SetStdout(io.Writer) Cmd
	SetStderr(io.Writer) Cmd
}

type Cmder interface {
	Command(string, ...string) Cmd
	CommandContext(context.Context, string, ...string) Cmd
}

type RunError struct {
	Command []string
	Output  []byte
	Inner   error
}

var _ error = &RunError{}

func (e *RunError) Error() string {
	return fmt.Sprintf("command \"%s\" failed with error: %v", e.PrettyCommand(), e.Inner)
}

func (e *RunError) PrettyCommand() string {
	return PrettyCommand(e.Command[0], e.Command[1:]...)
}

func (e *RunError) Cause() error {
	if e.Inner != nil {
		return e.Inner
	}
	return e
}
