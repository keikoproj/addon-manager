package cli

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/keikoproj/addon-manager/test-load/pkg/log"
)

type Logger struct {
	writer        io.Writer
	writerMu      sync.Mutex
	verbosity     log.Level
	bufferPool    *bufferPool
	isSmartWriter bool
}

var _ log.Logger = &Logger{}

func NewLogger(writer io.Writer, verbosity log.Level) *Logger {
	l := &Logger{
		verbosity:  verbosity,
		bufferPool: newBufferPool(),
	}
	l.SetWriter(writer)
	return l
}

func (l *Logger) SetWriter(w io.Writer) {
	l.writerMu.Lock()
	defer l.writerMu.Unlock()
	l.writer = w
	l.isSmartWriter = false
}

func (l *Logger) ColorEnabled() bool {
	l.writerMu.Lock()
	defer l.writerMu.Unlock()
	return l.isSmartWriter
}

func (l *Logger) getVerbosity() log.Level {
	return log.Level(atomic.LoadInt32((*int32)(&l.verbosity)))
}

func (l *Logger) SetVerbosity(verbosity log.Level) {
	atomic.StoreInt32((*int32)(&l.verbosity), int32(verbosity))
}

func (l *Logger) write(p []byte) (n int, err error) {
	l.writerMu.Lock()
	defer l.writerMu.Unlock()
	return l.writer.Write(p)
}

func (l *Logger) writeBuffer(buf *bytes.Buffer) {
	if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	_, _ = l.write(buf.Bytes())
}

func (l *Logger) print(message string) {
	buf := bytes.NewBufferString(message)
	l.writeBuffer(buf)
}

func (l *Logger) printf(format string, args ...interface{}) {
	buf := l.bufferPool.Get()
	fmt.Fprintf(buf, format, args...)
	l.writeBuffer(buf)
	l.bufferPool.Put(buf)
}

func addDebugHeader(buf *bytes.Buffer) {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		file = "???"
		line = 1
	} else {
		if slash := strings.LastIndex(file, "/"); slash >= 0 {
			path := file
			file = path[slash+1:]
			if dirsep := strings.LastIndex(path[:slash], "/"); dirsep >= 0 {
				file = path[dirsep+1:]
			}
		}
	}
	buf.Grow(len(file) + 11) // we know at least this many bytes are needed
	buf.WriteString("DEBUG: ")
	buf.WriteString(file)
	buf.WriteByte(':')
	fmt.Fprintf(buf, "%d", line)
	buf.WriteByte(']')
	buf.WriteByte(' ')
}

// debug is like print but with a debug log header
func (l *Logger) debug(message string) {
	buf := l.bufferPool.Get()
	addDebugHeader(buf)
	buf.WriteString(message)
	l.writeBuffer(buf)
	l.bufferPool.Put(buf)
}

// debugf is like printf but with a debug log header
func (l *Logger) debugf(format string, args ...interface{}) {
	buf := l.bufferPool.Get()
	addDebugHeader(buf)
	fmt.Fprintf(buf, format, args...)
	l.writeBuffer(buf)
	l.bufferPool.Put(buf)
}

// Warn is part of the log.Logger interface
func (l *Logger) Warn(message string) {
	l.print(message)
}

// Warnf is part of the log.Logger interface
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.printf(format, args...)
}

// Error is part of the log.Logger interface
func (l *Logger) Error(message string) {
	l.print(message)
}

// Errorf is part of the log.Logger interface
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.printf(format, args...)
}

// V is part of the log.Logger interface
func (l *Logger) V(level log.Level) log.InfoLogger {
	return infoLogger{
		logger:  l,
		level:   level,
		enabled: level <= l.getVerbosity(),
	}
}

// infoLogger implements log.InfoLogger for Logger
type infoLogger struct {
	logger  *Logger
	level   log.Level
	enabled bool
}

// Enabled is part of the log.InfoLogger interface
func (i infoLogger) Enabled() bool {
	return i.enabled
}

// Info is part of the log.InfoLogger interface
func (i infoLogger) Info(message string) {
	if !i.enabled {
		return
	}
	// for > 0, we are writing debug messages, include extra info
	if i.level > 0 {
		i.logger.debug(message)
	} else {
		i.logger.print(message)
	}
}

// Infof is part of the log.InfoLogger interface
func (i infoLogger) Infof(format string, args ...interface{}) {
	if !i.enabled {
		return
	}
	// for > 0, we are writing debug messages, include extra info
	if i.level > 0 {
		i.logger.debugf(format, args...)
	} else {
		i.logger.printf(format, args...)
	}
}

// bufferPool is a type safe sync.Pool of *byte.Buffer, guaranteed to be Reset
type bufferPool struct {
	sync.Pool
}

// newBufferPool returns a new bufferPool
func newBufferPool() *bufferPool {
	return &bufferPool{
		sync.Pool{
			New: func() interface{} {
				// The Pool's New function should generally only return pointer
				// types, since a pointer can be put into the return interface
				// value without an allocation:
				return new(bytes.Buffer)
			},
		},
	}
}

// Get obtains a buffer from the pool
func (b *bufferPool) Get() *bytes.Buffer {
	return b.Pool.Get().(*bytes.Buffer)
}

// Put returns a buffer to the pool, resetting it first
func (b *bufferPool) Put(x *bytes.Buffer) {
	// only store small buffers to avoid pointless allocation
	// avoid keeping arbitrarily large buffers
	if x.Len() > 256 {
		return
	}
	x.Reset()
	b.Pool.Put(x)
}
