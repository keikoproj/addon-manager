package app

import (
	"os"

	"github.com/keikoproj/addon-manager/test-load/pkg/cmd"
	"github.com/keikoproj/addon-manager/test-load/pkg/cmd/dolphin"
	"github.com/keikoproj/addon-manager/test-load/pkg/exec"
	"github.com/keikoproj/addon-manager/test-load/pkg/log"
)

func Main() {
	if err := Run(cmd.NewLogger(), cmd.StandardIOStreams(), os.Args[1:]); err != nil {
		os.Exit(1)
	}
}

func Run(logger log.Logger, streams cmd.IOStreams, args []string) error {
	c := dolphin.NewCommand(logger, streams)
	c.SetArgs(args)
	if err := c.Execute(); err != nil {
		logError(logger, err)
		return err
	}
	return nil
}

func logError(logger log.Logger, err error) {
	logger.Errorf("ERROR: %v", err)
	if err := exec.RunErrorForError(err); err != nil {
		logger.Errorf("\nCommand Output: %s", err.Output)
	}
}
