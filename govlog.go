package governor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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
	case "STDOUT":
		return os.Stdout
	case "TEST":
		return nil
	default:
		return os.Stdout
	}
}

type (
	zerologSerializer struct {
		log     *zerolog.Logger
		isDebug bool
	}
)

func newZerologSerializer(c Config) klog.Serializer {
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
	w := logOutputFromString(c.logOutput)
	if w == nil {
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
		status int
	}
)

func (w *govResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (s *Server) reqLoggerMiddleware(next http.Handler) http.Handler {
	reqcount := atomic.Uint32{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		lreqid := s.lreqID(reqcount.Add(1))
		c.Set(ctxKeyLocalReqID{}, lreqid)
		var forwarded string
		if ip := c.RealIP(); ip != nil {
			forwarded = ip.String()
		}
		c.LogFields(klog.Fields{
			"http.host":      c.Req().Host,
			"http.method":    c.Req().Method,
			"http.reqpath":   c.Req().URL.EscapedPath(),
			"http.remote":    c.Req().RemoteAddr,
			"http.forwarded": forwarded,
			"http.lreqid":    lreqid,
		})
		if reqIsWS(c.Req()) {
			s.log.Info(c.Ctx(), "WS open", klog.Fields{
				"ws": true,
			})
			start := time.Now()
			next.ServeHTTP(c.R())
			duration := time.Since(start)
			route := chi.RouteContext(c.Ctx()).RoutePattern()
			s.log.Info(c.Ctx(), "WS close", klog.Fields{
				"http.ws":          true,
				"http.route":       route,
				"http.duration_ms": duration.Milliseconds(),
			})
		} else {
			s.log.Info(c.Ctx(), "HTTP request", klog.Fields{
				"http.ws": false,
			})
			start := time.Now()
			w2 := &govResponseWriter{
				ResponseWriter: w,
				status:         0,
			}
			next.ServeHTTP(w2, c.Req())
			duration := time.Since(start)
			route := chi.RouteContext(c.Ctx()).RoutePattern()
			s.log.Info(c.Ctx(), "HTTP response", klog.Fields{
				"http.ws":         false,
				"http.route":      route,
				"http.status":     w2.status,
				"http.latency_us": duration.Microseconds(),
			})
		}
	})
}
