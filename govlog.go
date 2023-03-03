package governor

import (
	"io"
	"os"

	"golang.org/x/exp/slog"
	"xorkevin.dev/klog"
)

func newLogger(c Config, cl configLogger) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(cl.level),
		klog.OptHandler(newLogHandler(cl)),
		klog.OptSubhandler("", []klog.Attr{
			slog.String("gov.appname", c.Appname),
			slog.String("gov.version", c.Version.String()),
			slog.String("gov.hostname", c.Hostname),
			slog.String("gov.instance", c.Instance),
		}),
	))
}

func logOutputFromString(s string) io.Writer {
	switch s {
	case "STDERR":
		return os.Stderr
	case "STDOUT":
		return os.Stdout
	default:
		return os.Stderr
	}
}

func newLogHandler(c configLogger) klog.Handler {
	w := c.writer
	if w == nil {
		w = logOutputFromString(c.output)
	}
	w = klog.NewSyncWriter(w)
	isDebug := c.level == klog.LevelDebug.String()
	if isDebug {
		return klog.NewTextSlogHandler(w)
	}
	return klog.NewJSONSlogHandler(w)
}
