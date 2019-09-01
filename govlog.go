package governor

import (
	"github.com/sirupsen/logrus"
	"io"
	"os"
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
	}

	govlogger struct {
		level  int
		logger *logrus.Logger
	}
)

func logrusLevelToLog(level int) logrus.Level {
	switch level {
	case levelDebug:
		return logrus.DebugLevel
	case levelInfo:
		return logrus.InfoLevel
	case levelWarn:
		return logrus.WarnLevel
	case levelError:
		return logrus.ErrorLevel
	case levelFatal:
		return logrus.FatalLevel
	case levelPanic:
		return logrus.PanicLevel
	default:
		return logrus.InfoLevel
	}
}

func newLogger(c Config) Logger {
	l := logrus.New()
	if c.IsDebug() {
		l.Formatter = &logrus.TextFormatter{}
	} else {
		l.Formatter = &logrus.JSONFormatter{}
	}
	l.Out = c.LogOutput
	l.Level = logrusLevelToLog(c.LogLevel)
	return &govlogger{
		level:  c.LogLevel,
		logger: l,
	}
}

func (l *govlogger) createFields(data map[string]string) logrus.Fields {
	now, _ := time.Now().MarshalText()
	fields := logrus.Fields{
		"logtime": string(now),
	}
	if data != nil {
		for k, v := range data {
			fields[k] = v
		}
	}
	return fields
}

// Debug logs a debug level message
//
// This message will only be logged when the server configuration is in debug
// mode.
func (l *govlogger) Debug(msg string, data map[string]string) {
	fields := l.createFields(data)
	l.logger.WithFields(fields).Debug(msg)
}

// Info logs an info level message
func (l *govlogger) Info(msg string, data map[string]string) {
	fields := l.createFields(data)
	l.logger.WithFields(fields).Info(msg)
}

// Warn logs a warning level message
func (l *govlogger) Warn(msg string, data map[string]string) {
	fields := l.createFields(data)
	l.logger.WithFields(fields).Warn(msg)
}

// Error logs a server error level message
func (l *govlogger) Error(msg string, data map[string]string) {
	fields := l.createFields(data)
	l.logger.WithFields(fields).Error(msg)
}

// Fatal logs a fatal error message then exits
func (l *govlogger) Fatal(msg string, data map[string]string) {
	fields := l.createFields(data)
	l.logger.WithFields(fields).Fatal(msg)
}
