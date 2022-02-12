package log

// Level is a verbosity logging level for Info logs
// Refer https://github.com/kubernetes/klog
type Level int32

// Logger defines the logging interface dolphin uses
// roughly a subset of github.com/kubernetes/klog
type Logger interface {
	// Warn should be used to write user facing warnings
	Warn(message string)
	// Warnf should be used to write Printf style user facing warnings
	Warnf(format string, args ...interface{})
	// Error may be used to write an error message when it occurs
	// Prefer returning an error instead in most cases
	Error(message string)
	// Errorf may be used to write a Printf style error message when it occurs
	// Prefer returning an error instead in most cases
	Errorf(format string, args ...interface{})
	// V() returns an InfoLogger for a given verbosity Level
	//
	// Normal verbosity levels:
	// V(0): normal user facing messages go to V(0)
	// V(1): debug messages start when V(N > 0), these should be high level
	// V(2): more detailed log messages
	// V(3+): trace level logging, in increasing "noisiness" ... allowing
	// arbitrarily detailed logging at extremely low cost unless the
	// logger has actually been configured to display these (E.G. via the -v
	// command line flag)
	//
	// It is expected that the returned InfoLogger will be extremely cheap
	// to interact with for a Level greater than the enabled level
	V(Level) InfoLogger
}

// InfoLogger defines the info logging interface dolphin uses
// roughly a subset of Verbose from github.com/kubernetes/klog
type InfoLogger interface {
	// Info is used to write a user facing status message
	//
	// See: Logger.V
	Info(message string)
	// Infof is used to write a Printf style user facing status message
	Infof(format string, args ...interface{})
	// Enabled should return true if this verbosity level is enabled
	// on the Logger
	//
	// See: Logger.V
	Enabled() bool
}
