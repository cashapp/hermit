package ui

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cashapp/hermit/errors"
)

//go:generate stringer -linecomment -type Level

// Level for a log message.
type Level int

// Log levels.
const (
	// LevelAuto will detect the log level from the environment via
	// HERMIT_LOG=<level>, DEBUG=1, then finally from flag.
	LevelAuto  Level = iota // auto
	LevelTrace              // trace
	LevelDebug              // debug
	LevelInfo               // info
	LevelWarn               // warn
	LevelError              // error
	LevelFatal              // fatal
)

var (
	ansiStripRe = regexp.MustCompile("[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))|[\n\r]")
	levelColor  = map[Level]string{
		LevelTrace: "\033[37m",
		LevelDebug: "\033[36m",
		LevelInfo:  "\033[32m",
		LevelWarn:  "\033[33m",
		LevelError: "\033[31m",
		LevelFatal: "\033[31m",
	}
)

// Visible returns true if "other" is visible.
func (l Level) Visible(other Level) bool {
	return other >= l
}

func (l *Level) UnmarshalText(text []byte) error {
	var err error
	*l, err = LevelFromString(string(text))
	return err
}

// LevelFromString maps a level to a string.
func LevelFromString(s string) (Level, error) {
	switch s {
	case "auto":
		return LevelAuto, nil
	case "trace":
		return LevelTrace, nil
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "fatal":
		return LevelFatal, nil
	default:
		return 0, errors.Errorf("invalid log level %q", s)
	}
}

// Logger interface.
type Logger interface {
	io.Writer
	Tracef(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	WriterAt(level Level) SyncWriter
}

// LogElapsed logs the duration of a function call. Use with defer:
//
//             defer LogElapsed(log, "something")()
func LogElapsed(log Logger, message string, args ...interface{}) func() {
	start := time.Now()
	return func() {
		args = append(args, time.Since(start))
		log.Tracef(message+" (%s elapsed)", args...)
	}
}

type logWriter struct {
	lock  sync.Mutex
	level Level
	buf   []byte
	logf  func(level Level, format string, args ...interface{})
}

func (l *logWriter) Sync() error {
	l.lock.Lock()
	if len(l.buf) > 0 {
		line := string(l.buf)
		l.buf = nil
		l.logf(l.level, "%s", ansiStripRe.ReplaceAllString(line, ""))
	}
	l.lock.Unlock()
	return nil
}

// Write to the logger with the logging prefix, if any.
func (l *logWriter) Write(b []byte) (int, error) {
	l.lock.Lock()
	l.buf = append(l.buf, b...)
	var lines []string
	for i := bytes.IndexByte(l.buf, '\n'); i != -1; i = bytes.IndexByte(l.buf, '\n') {
		lines = append(lines, string(l.buf[:i]))
		if i >= len(l.buf) {
			l.buf = nil
			break
		}
		l.buf = l.buf[i+1:]
	}
	l.lock.Unlock()
	for _, line := range lines {
		l.logf(l.level, "%s", ansiStripRe.ReplaceAllString(line, ""))
	}
	return len(b), nil
}

type loggingMixin struct {
	logWriter
	task    string
	subtask string
	logf    func(level Level, label string, format string, args ...interface{})
}

func (l *loggingMixin) WriterAt(level Level) SyncWriter {
	return &logWriter{
		level: level,
		logf: func(level Level, format string, args ...interface{}) {
			l.logf(level, l.label(), format, args...)
		},
	}
}

func (l *loggingMixin) label() string {
	parts := make([]string, 0, 2)
	if l.task != "" {
		parts = append(parts, l.task)
	}
	if l.subtask != "" {
		parts = append(parts, l.subtask)
	}
	return strings.Join(parts, ":")
}

// Tracef logs a message at trace level.
func (l *loggingMixin) Tracef(format string, args ...interface{}) {
	l.logf(LevelTrace, l.label(), format, args...)
}

// Debugf logs a message at debug level.
func (l *loggingMixin) Debugf(format string, args ...interface{}) {
	l.logf(LevelDebug, l.label(), format, args...)
}

// Infof logs a message at info level.
func (l *loggingMixin) Infof(format string, args ...interface{}) {
	l.logf(LevelInfo, l.label(), format, args...)
}

// Warnf logs a message at warning level.
func (l *loggingMixin) Warnf(format string, args ...interface{}) {
	l.logf(LevelWarn, l.label(), format, args...)
}

// Errorf logs a message at error level.
func (l *loggingMixin) Errorf(format string, args ...interface{}) {
	l.logf(LevelError, l.label(), format, args...)
}

// Fatalf logs a fatal message and exits with a non-zero status.
//
// Additionally, log output will not be cleared.
func (l *loggingMixin) Fatalf(format string, args ...interface{}) {
	l.logf(LevelFatal, l.label(), format, args...)
}

// AutoLevel sets the log level from environment variables if set to LevelAuto.
func AutoLevel(level Level) Level {
	if level != LevelAuto {
		return level
	}
	if envLevel := os.Getenv("HERMIT_LOG"); envLevel != "" {
		if err := level.UnmarshalText([]byte(envLevel)); err == nil {
			return level
		}
	} else if os.Getenv("DEBUG") != "" {
		return LevelTrace
	}
	return LevelInfo
}
