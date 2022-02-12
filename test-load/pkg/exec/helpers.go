package exec

import (
	"bufio"
	"bytes"
	"os"
	"strings"

	"github.com/alessio/shellescape"
)

// PrettyCommand...
func PrettyCommand(name string, args ...string) string {
	var out strings.Builder
	out.WriteString(shellescape.Quote(name))
	for _, arg := range args {
		out.WriteByte(' ')
		out.WriteString(shellescape.Quote(arg))
	}
	return out.String()
}

// RunErrorForError returns a RunError if the error contains a RunError.
// Otherwise it returns nil
func RunErrorForError(err error) *RunError {
	var runError *RunError
	for {
		if rErr, ok := err.(*RunError); ok {
			runError = rErr
		} else {
			break
		}
	}
	return runError
}

// CombinedOutputLines is like os/exec's cmd.CombinedOutput(),
// but over our Cmd interface, and instead of returning the byte buffer of
// stderr + stdout, it scans these for lines and returns a slice of output lines
func CombinedOutputLines(cmd Cmd) (lines []string, err error) {
	var buff bytes.Buffer
	cmd.SetStdout(&buff)
	cmd.SetStderr(&buff)
	err = cmd.Run()
	scanner := bufio.NewScanner(&buff)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, err
}

// OutputLines is like os/exec's cmd.Output(),
// but over our Cmd interface, and instead of returning the byte buffer of
// stdout, it scans these for lines and returns a slice of output lines
func OutputLines(cmd Cmd) (lines []string, err error) {
	var buff bytes.Buffer
	cmd.SetStdout(&buff)
	err = cmd.Run()
	scanner := bufio.NewScanner(&buff)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, err
}

// Output is like os/exec's cmd.Output, but over our Cmd interface
func Output(cmd Cmd) ([]byte, error) {
	var buff bytes.Buffer
	cmd.SetStdout(&buff)
	err := cmd.Run()
	return buff.Bytes(), err
}

// InheritOutput sets cmd's output to write to the current process's stdout and stderr
func InheritOutput(cmd Cmd) Cmd {
	cmd.SetStderr(os.Stderr)
	cmd.SetStdout(os.Stdout)
	return cmd
}
