package governor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"xorkevin.dev/klog"
)

func newLogger(c Config, cl configLogger) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(cl.level),
		klog.OptSerializer(newZerologSerializer(cl)),
		klog.OptFields(klog.Fields{
			"gov.appname":  c.Appname,
			"gov.version":  c.Version.String(),
			"gov.hostname": c.Hostname,
			"gov.instance": c.Instance,
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

type (
	zerologSerializer struct {
		log     *zerolog.Logger
		isDebug bool
	}
)

var (
	zerologInitOnce = &sync.Once{}
)

func setZerologGlobals() {
	zerolog.LevelFieldName = "level"
	zerolog.LevelDebugValue = klog.LevelDebug.String()
	zerolog.LevelInfoValue = klog.LevelInfo.String()
	zerolog.LevelWarnValue = klog.LevelWarn.String()
	zerolog.LevelErrorValue = klog.LevelError.String()
	zerolog.TimestampFieldName = "time"
	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.CallerFieldName = "caller"
	zerolog.MessageFieldName = "msg"
	zerolog.ErrorFieldName = "error"
	zerolog.ErrorStackFieldName = "stacktrace"
}

func newZerologSerializer(c configLogger) klog.Serializer {
	zerologInitOnce.Do(setZerologGlobals)
	w := c.writer
	if w == nil {
		w = logOutputFromString(c.output)
	}
	w = klog.NewSyncWriter(w)
	isDebug := c.level == klog.LevelDebug.String()
	if isDebug {
		w = zerolog.NewConsoleWriter(func(cw *zerolog.ConsoleWriter) {
			cw.Out = w
		})
	}
	l := zerolog.New(w)
	return &zerologSerializer{
		log:     &l,
		isDebug: isDebug,
	}
}

var (
	reservedLogFields = map[string]struct{}{
		"level":      {},
		"time":       {},
		"unixtimeus": {},
		"caller":     {},
		"path":       {},
		"msg":        {},
	}
	noDebugLogFields = map[string]struct{}{
		"gov.appname":  {},
		"gov.version":  {},
		"gov.hostname": {},
		"gov.instance": {},
	}
)

func (s *zerologSerializer) Log(level klog.Level, t, mt time.Time, caller *klog.Frame, path string, msg string, fields klog.Fields) {
	timestr := t.Format(time.RFC3339Nano)
	callerstr := ""
	if caller != nil {
		if s.isDebug {
			callerstr = fmt.Sprintf("%s:%d", caller.File, caller.Line)
		} else {
			callerstr = caller.String()
		}
	}
	if path == "" {
		path = "."
	}
	for k := range reservedLogFields {
		delete(fields, k)
	}
	if s.isDebug && msg != "Starting server" {
		for k := range noDebugLogFields {
			delete(fields, k)
		}
	}
	e := s.log.Info()
	switch level {
	case klog.LevelDebug:
		e = s.log.Debug()
	case klog.LevelInfo:
		e = s.log.Info()
	case klog.LevelWarn:
		e = s.log.Warn()
	case klog.LevelError:
		e = s.log.Error()
	}
	e = e.Str("level", level.String()).
		Str("time", timestr)
	if !s.isDebug {
		unixtimeus := t.UnixMicro()
		monotimestr := t.Format(time.RFC3339Nano)
		monounixtimeus := t.UnixMicro()
		e = e.Int64("unixtimeus", unixtimeus)
		e = e.Str("monotime", monotimestr)
		e = e.Int64("monounixtimeus", monounixtimeus)
	}
	e.Str("caller", callerstr).
		Str("path", path).
		Fields(map[string]interface{}(fields)).
		Msg(msg)
}

func newPlaintextLogger(c ClientConfig) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(c.logger.level),
		klog.OptSerializer(newPlaintextSerializer(c)),
	))
}

type (
	plaintextSerializer struct {
		w io.Writer
	}
)

func newPlaintextSerializer(c ClientConfig) klog.Serializer {
	w := c.LogWriter
	if w == nil {
		w = logOutputFromString(c.logger.output)
	}
	return &plaintextSerializer{
		w: klog.NewSyncWriter(w),
	}
}

func (s *plaintextSerializer) Log(level klog.Level, t, mt time.Time, caller *klog.Frame, path string, msg string, fields klog.Fields) {
	timestr := t.Format(time.RFC3339Nano)
	callerstr := ""
	if caller != nil {
		callerstr = fmt.Sprintf("%s:%d", caller.File, caller.Line)
	}
	if path == "" {
		path = "."
	}
	var b bytes.Buffer
	j := json.NewEncoder(&b)
	j.SetEscapeHTML(false)
	if err := j.Encode(fields); err != nil {
		// ignore marshal error
		return
	}
	fmt.Fprintf(s.w, "[%s %s] %s [%s %s] ", level.String(), timestr, msg, path, callerstr)
	if _, err := io.Copy(s.w, &b); err != nil {
		// ignore buffer copy error
		return
	}
}
