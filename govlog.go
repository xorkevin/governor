package governor

import (
	"github.com/sirupsen/logrus"
	"os"
)

const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
	levelFatal
	levelPanic
	levelNoLog
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

type (
	Logger interface {
		Debug(msg string, data map[string]string)
		Info(msg string, data map[string]string)
		Warn(msg string, data map[string]string)
		Error(msg string, data map[string]string)
		Fatal(msg string, data map[string]string)
	}

	govlogger struct {
	}
)

func levelToLog(level int) logrus.Level {
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

func newLogger(c Config) *logrus.Logger {
	l := logrus.New()
	if c.IsDebug() {
		l.Formatter = &logrus.TextFormatter{}
	} else {
		l.Formatter = &logrus.JSONFormatter{}
	}
	l.Out = os.Stdout
	l.Level = levelToLog(c.LogLevel)
	return l
}

func NewLogger(c Config) Logger {
	return &govlogger{}
}

func (l *govlogger) Debug(msg string, data map[string]string) {
}
func (l *govlogger) Info(msg string, data map[string]string) {
}
func (l *govlogger) Warn(msg string, data map[string]string) {
}
func (l *govlogger) Error(msg string, data map[string]string) {
}
func (l *govlogger) Fatal(msg string, data map[string]string) {
}
