package governor

import (
	"github.com/rs/zerolog"
	"io"
	"os"
	"strconv"
	"time"
)

const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
	levelFatal
	levelPanic
)

func envToLevel(e string) int {
	switch e {
	case "DEBUG":
		return levelDebug
	case "INFO":
		return levelInfo
	case "WARN":
		return levelWarn
	case "ERROR":
		return levelError
	case "FATAL":
		return levelFatal
	case "PANIC":
		return levelPanic
	default:
		return levelInfo
	}
}

func envToLogOutput(e string) io.Writer {
	switch e {
	case "STDOUT":
		return os.Stdout
	default:
		return os.Stdout
	}
}

type (
	// Logger is a governor logging interface that may write logs to a
	// configurable io.Writer
	Logger interface {
		Debug(msg string, data map[string]string)
		Info(msg string, data map[string]string)
		Warn(msg string, data map[string]string)
		Error(msg string, data map[string]string)
		Fatal(msg string, data map[string]string)
		Subtree(module string) Logger
		WithData(data map[string]string) Logger
	}

	govlogger struct {
		level  int
		logger *zerolog.Logger
		module string
		data   map[string]string
	}
)

func zerologLevelToLog(level int) zerolog.Level {
	switch level {
	case levelDebug:
		return zerolog.DebugLevel
	case levelInfo:
		return zerolog.InfoLevel
	case levelWarn:
		return zerolog.WarnLevel
	case levelError:
		return zerolog.ErrorLevel
	case levelFatal:
		return zerolog.FatalLevel
	case levelPanic:
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

type (
	zerologTimestampHook struct{}
)

func (h zerologTimestampHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	now := time.Now().Round(0)
	nowStr := now.Format(time.RFC3339)
	nowUnix := strconv.FormatInt(now.Unix(), 10)
	e.Str("time", nowStr)
	e.Str("unixtime", nowUnix)
}

func newLogger(c Config) Logger {
	zerolog.MessageFieldName = "msg"
	w := c.logOutput
	if c.IsDebug() {
		w = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = c.logOutput
		})
	}
	l := zerolog.New(w).Level(zerologLevelToLog(c.logLevel)).Hook(zerologTimestampHook{})
	return &govlogger{
		level:  c.logLevel,
		logger: &l,
		module: "",
		data:   nil,
	}
}

func (l *govlogger) withFields(e *zerolog.Event, msg string, data map[string]string) {
	if l.module != "" {
		e.Str("module", l.module)
	} else {
		e.Str("module", "root")
	}
	for k, v := range l.data {
		e.Str(k, v)
	}
	for k, v := range data {
		e.Str(k, v)
	}
	e.Msg(msg)
}

func (l *govlogger) Subtree(module string) Logger {
	m := l.module
	if m != "" {
		m += "."
	}
	return &govlogger{
		level:  l.level,
		logger: l.logger,
		module: m + module,
		data:   l.data,
	}
}

func (l *govlogger) WithData(data map[string]string) Logger {
	nextData := make(map[string]string, len(data)+len(l.data))
	if l.data != nil {
		for k, v := range l.data {
			nextData[k] = v
		}
	}
	if data != nil {
		for k, v := range data {
			nextData[k] = v
		}
	}
	return &govlogger{
		level:  l.level,
		logger: l.logger,
		module: l.module,
		data:   nextData,
	}
}

// Debug logs a debug level message
//
// This message will only be logged when the server configuration is in debug
// mode.
func (l *govlogger) Debug(msg string, data map[string]string) {
	l.withFields(l.logger.Debug(), msg, data)
}

// Info logs an info level message
func (l *govlogger) Info(msg string, data map[string]string) {
	l.withFields(l.logger.Info(), msg, data)
}

// Warn logs a warning level message
func (l *govlogger) Warn(msg string, data map[string]string) {
	l.withFields(l.logger.Warn(), msg, data)
}

// Error logs a server error level message
func (l *govlogger) Error(msg string, data map[string]string) {
	l.withFields(l.logger.Error(), msg, data)
}

// Fatal logs a fatal error message then exits
func (l *govlogger) Fatal(msg string, data map[string]string) {
	l.withFields(l.logger.Fatal(), msg, data)
}
