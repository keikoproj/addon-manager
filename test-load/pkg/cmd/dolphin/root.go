package dolphin

import (
	"io"
	"io/ioutil"

	"github.com/keikoproj/addon-manager/test-load/pkg/cmd"
	"github.com/keikoproj/addon-manager/test-load/pkg/cmd/dolphin/tests"
	"github.com/keikoproj/addon-manager/test-load/pkg/log"
	"github.com/spf13/cobra"
)

type flagpole struct {
	LogLevel  string
	Verbosity int32
	Quiet     bool
}

// NewCommand returns a new cobra.Command implementing the root command for dolphin
func NewCommand(logger log.Logger, streams cmd.IOStreams) *cobra.Command {
	flags := &flagpole{}
	cmd := &cobra.Command{
		Args:  cobra.NoArgs,
		Use:   "dolphin",
		Short: "dolphin is a tool for managing local Kubernetes clusters pods",
		Long:  "dolphin creates and manages local Kubernetes clusters using Docker container 'nodes'",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return runE(logger, flags, cmd)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		// Version:       version.Version(),
	}
	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)
	cmd.PersistentFlags().StringVar(
		&flags.LogLevel,
		"loglevel",
		"",
		"DEPRECATED: see -v instead",
	)
	cmd.PersistentFlags().Int32VarP(
		&flags.Verbosity,
		"verbosity",
		"v",
		0,
		"info log verbosity, higher value produces more output",
	)
	cmd.PersistentFlags().BoolVarP(
		&flags.Quiet,
		"quiet",
		"q",
		false,
		"silence all stderr output",
	)
	// add all top level subcommands
	cmd.AddCommand(tests.NewCommand(logger, streams))
	return cmd
}

func runE(logger log.Logger, flags *flagpole, command *cobra.Command) error {
	// handle limited migration for --loglevel
	setLogLevel := command.Flag("loglevel").Changed
	setVerbosity := command.Flag("verbosity").Changed
	if setLogLevel && !setVerbosity {
		switch flags.LogLevel {
		case "debug":
			flags.Verbosity = 3
		case "trace":
			flags.Verbosity = 2147483647
		}
	}
	// normal logger setup
	if flags.Quiet {
		maybeSetWriter(logger, ioutil.Discard)
	}
	maybeSetVerbosity(logger, log.Level(flags.Verbosity))
	if setLogLevel {
		logger.Warn("WARNING: --loglevel is deprecated, please switch to -v and -q!")
	}
	return nil
}

// maybeSetWriter will call logger.SetWriter(w) if logger has a SetWriter method
func maybeSetWriter(logger log.Logger, w io.Writer) {
	type writerSetter interface {
		SetWriter(io.Writer)
	}
	v, ok := logger.(writerSetter)
	if ok {
		v.SetWriter(w)
	}
}

// maybeSetVerbosity will call logger.SetVerbosity(verbosity) if logger
// has a SetVerbosity method
func maybeSetVerbosity(logger log.Logger, verbosity log.Level) {
	type verboser interface {
		SetVerbosity(log.Level)
	}
	v, ok := logger.(verboser)
	if ok {
		v.SetVerbosity(verbosity)
	}
}
