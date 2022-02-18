package exec

import (
	"bufio"
	"bytes"
	"os"
	"strings"

	"github.com/alessio/shellescape"
)

func PrettyCommand(name string, args ...string) string {
	var out strings.Builder
	out.WriteString(shellescape.Quote(name))
	for _, arg := range args {
		out.WriteByte(' ')
		out.WriteString(shellescape.Quote(arg))
	}
	return out.String()
}

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

func Output(cmd Cmd) ([]byte, error) {
	var buff bytes.Buffer
	cmd.SetStdout(&buff)
	err := cmd.Run()
	return buff.Bytes(), err
}

func InheritOutput(cmd Cmd) Cmd {
	cmd.SetStderr(os.Stderr)
	cmd.SetStdout(os.Stdout)
	return cmd
}
