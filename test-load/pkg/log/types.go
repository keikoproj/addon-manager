package log

// Level is a verbosity logging level for Info logs
// Refer https://github.com/kubernetes/klog
type Level int32

type Logger interface {
	Warn(message string)
	Warnf(format string, args ...interface{})
	Error(message string)
	Errorf(format string, args ...interface{})
	V(Level) InfoLogger
}

type InfoLogger interface {
	Info(message string)
	Infof(format string, args ...interface{})
	Enabled() bool
}
