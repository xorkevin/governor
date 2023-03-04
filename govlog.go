package governor

import (
	"io"
	"os"

	"xorkevin.dev/klog"
)

func newLogger(c Config, cl configLogger) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(cl.level),
		klog.OptHandler(newLogHandler(cl)),
		klog.OptSubhandler("",
			klog.AGroup("gov",
				klog.AString("appname", c.Appname),
				klog.AString("version", c.Version.String()),
				klog.AString("hostname", c.Hostname),
				klog.AString("instance", c.Instance),
			),
		),
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

func newPlaintextLogger(c configLogger) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(c.level),
		klog.OptHandler(newPlaintextLogHandler(c)),
	))
}

func newPlaintextLogHandler(c configLogger) klog.Handler {
	w := c.writer
	if w == nil {
		w = logOutputFromString(c.output)
	}
	w = klog.NewSyncWriter(w)
	return klog.NewTextSlogHandler(w)
}
