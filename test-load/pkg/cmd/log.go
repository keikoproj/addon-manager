package cmd

import (
	"io"
	"os"

	"github.com/keikoproj/addon-manager/test-load/pkg/internal/cli"
	"github.com/keikoproj/addon-manager/test-load/pkg/log"
)

// NewLogger returns the standard logger used by the dolphin CLI
// This logger writes to os.Stderr
func NewLogger() log.Logger {
	var writer io.Writer = os.Stderr
	return cli.NewLogger(writer, 0)
}
