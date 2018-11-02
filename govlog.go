package governor

import (
	"github.com/sirupsen/logrus"
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
		Debug(msg, module, event string, code int, data map[string]string)
		Info(msg, module, event string, code int, data map[string]string)
		Warn(msg, module, event string, code int, data map[string]string)
		Error(msg, module, event string, code int, data map[string]string)
		Fatal(msg, module, event string, code int, data map[string]string)
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

func newLogger(c Config) *logrus.Logger {
	l := logrus.New()
	if c.IsDebug() {
		l.Formatter = &logrus.TextFormatter{}
	} else {
		l.Formatter = &logrus.JSONFormatter{}
	}
	l.Out = os.Stdout
	l.Level = logrusLevelToLog(c.LogLevel)
	return l
}

func NewLogger(c Config) Logger {
	l := logrus.New()
	if c.IsDebug() {
		l.Formatter = &logrus.TextFormatter{}
	} else {
		l.Formatter = &logrus.JSONFormatter{}
	}
	l.Out = os.Stdout
	l.Level = logrusLevelToLog(c.LogLevel)
	return &govlogger{
		level:  c.LogLevel,
		logger: l,
	}
}

func (l *govlogger) createFields(module, event string, code int, data map[string]string) logrus.Fields {
	now, _ := time.Now().MarshalText()
	fields := logrus.Fields{
		"time":   now,
		"module": module,
		"event":  event,
	}
	if code > 0 {
		fields["code"] = code
	}
	if data != nil {
		for k, v := range data {
			fields[k] = v
		}
	}
	return fields
}

func (l *govlogger) Debug(msg, module, event string, code int, data map[string]string) {
	fields := l.createFields(module, event, code, data)
	l.logger.WithFields(fields).Debug(msg)
}

func (l *govlogger) Info(msg, module, event string, code int, data map[string]string) {
	fields := l.createFields(module, event, code, data)
	l.logger.WithFields(fields).Info(msg)
}

func (l *govlogger) Warn(msg, module, event string, code int, data map[string]string) {
	fields := l.createFields(module, event, code, data)
	l.logger.WithFields(fields).Warn(msg)
}

func (l *govlogger) Error(msg, module, event string, code int, data map[string]string) {
	fields := l.createFields(module, event, code, data)
	l.logger.WithFields(fields).Error(msg)
}

func (l *govlogger) Fatal(msg, module, event string, code int, data map[string]string) {
	fields := l.createFields(module, event, code, data)
	l.logger.WithFields(fields).Fatal(msg)
}
