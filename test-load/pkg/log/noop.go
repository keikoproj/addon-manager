package log

// NoopLogger implements the Logger interface and never logs anything
type NoopLogger struct{}

// Warn meets the Logger interface but does nothing
func (n NoopLogger) Warn(message string) {}

// Warnf meets the Logger interface but does nothing
func (n NoopLogger) Warnf(format string, args ...interface{}) {}

// Error meets the Logger interface but does nothing
func (n NoopLogger) Error(message string) {}

// Errorf meets the Logger interface but does nothing
func (n NoopLogger) Errorf(format string, args ...interface{}) {}

// V meets the Logger interface but does nothing
func (n NoopLogger) V(level Level) InfoLogger { return NoopInfoLogger{} }

// NoopInfoLogger implements the InfoLogger interface and never logs anything
type NoopInfoLogger struct{}

// Enabled meets the InfoLogger interface but always returns false
func (n NoopInfoLogger) Enabled() bool { return false }

// Info meets the InfoLogger interface but does nothing
func (n NoopInfoLogger) Info(message string) {}

// Infof meets the InfoLogger interface but does nothing
func (n NoopInfoLogger) Infof(format string, args ...interface{}) {}
