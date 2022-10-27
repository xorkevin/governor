package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/klog"
)

func newLogger(c Config) *klog.LevelLogger {
	return klog.NewLevelLogger(klog.New(
		klog.OptMinLevelStr(c.logLevel),
		klog.OptSerializer(newZerologSerializer(c)),
		klog.OptFields(klog.Fields{
			"gov.appname":  c.appname,
			"gov.version":  c.version.String(),
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

func newZerologSerializer(c Config) klog.Serializer {
	zerologInitOnce.Do(setZerologGlobals)
	w := logOutputFromString(c.logOutput)
	if c.logWriter != nil {
		w = c.logWriter
	}
	w = klog.NewSyncWriter(w)
	isDebug := c.logLevel == klog.LevelDebug.String()
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
		klog.OptMinLevelStr(c.logLevel),
		klog.OptSerializer(newPlaintextSerializer(c)),
	))
}

type (
	plaintextSerializer struct {
		w io.Writer
	}
)

func newPlaintextSerializer(c ClientConfig) klog.Serializer {
	w := logOutputFromString(c.logOutput)
	if c.logWriter != nil {
		w = c.logWriter
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
	fmt.Fprintf(s.w, "[%s %s] %s %s %s ", level.String(), timestr, msg, path, callerstr)
	if _, err := io.Copy(s.w, &b); err != nil {
		// ignore buffer copy error
		return
	}
}

type (
	ctxKeyLocalReqID struct{}
)

func getCtxLocalReqID(ctx context.Context) string {
	v := ctx.Value(ctxKeyLocalReqID{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func (s *Server) lreqID(count uint32) string {
	return s.config.Instance + "-" + uid.ReqID(count)
}

type (
	govResponseWriter struct {
		http.ResponseWriter
		status      int
		wroteHeader bool
	}
)

func (w *govResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *govResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *govResponseWriter) isWS() bool {
	return w.status == http.StatusSwitchingProtocols
}

func (s *Server) reqLoggerMiddleware(next http.Handler) http.Handler {
	reqcount := atomic.Uint32{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		lreqid := s.lreqID(reqcount.Add(1))
		c.Set(ctxKeyLocalReqID{}, lreqid)
		var realip string
		if ip := c.RealIP(); ip != nil {
			realip = ip.String()
		}
		c.LogFields(klog.Fields{
			"http.host":    c.Req().Host,
			"http.method":  c.Req().Method,
			"http.reqpath": c.Req().URL.EscapedPath(),
			"http.remote":  c.Req().RemoteAddr,
			"http.realip":  realip,
			"http.lreqid":  lreqid,
		})
		w2 := &govResponseWriter{
			ResponseWriter: w,
			status:         0,
		}
		s.log.Info(c.Ctx(), "HTTP request", nil)
		start := time.Now()
		next.ServeHTTP(w2, c.Req())
		duration := time.Since(start)
		route := chi.RouteContext(c.Ctx()).RoutePattern()
		if w2.isWS() {
			s.log.Info(c.Ctx(), "WS close", klog.Fields{
				"http.ws":          true,
				"http.route":       route,
				"http.status":      w2.status,
				"http.duration_ms": duration.Milliseconds(),
			})
		} else {
			s.log.Info(c.Ctx(), "HTTP response", klog.Fields{
				"http.ws":         false,
				"http.route":      route,
				"http.status":     w2.status,
				"http.latency_us": duration.Microseconds(),
			})
		}
	})
}
