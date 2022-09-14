package governor

import (
	"context"
	"encoding/base32"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
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
		log *zerolog.Logger
	}
)

func levelToZerologLevel(level klog.Level) zerolog.Level {
	switch level {
	case klog.LevelDebug:
		return zerolog.DebugLevel
	case klog.LevelInfo:
		return zerolog.InfoLevel
	case klog.LevelWarn:
		return zerolog.WarnLevel
	case klog.LevelError:
		return zerolog.ErrorLevel
	case klog.LevelNone:
		return zerolog.Disabled
	default:
		return zerolog.InfoLevel
	}
}

func newZerologSerializer(c Config) klog.Serializer {
	zerolog.LevelFieldName = "level"
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
	if c.logLevel == klog.LevelDebug.String() {
		w = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
			w.Out = w
		})
	}
	l := zerolog.New(w).Level(levelToZerologLevel(klog.LevelFromString(c.logLevel)))
	return &zerologSerializer{
		log: &l,
	}
}

var (
	reservedLogFields = map[string]struct{}{
		"level":      {},
		"time":       {},
		"unixtime":   {},
		"unixtimeus": {},
		"caller":     {},
		"path":       {},
		"msg":        {},
	}
)

func (s *zerologSerializer) Log(level klog.Level, t time.Time, caller *klog.Frame, path string, msg string, fields klog.Fields) {
	timestr := t.Format(time.RFC3339Nano)
	unixtime := t.Unix()
	unixtimeus := t.UnixMicro()
	callerstr := ""
	if caller != nil {
		callerstr = caller.String()
	}
	for k := range reservedLogFields {
		delete(fields, k)
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
	e.Str("level", level.String()).
		Str("time", timestr).
		Int64("unixtime", unixtime).
		Int64("unixtimeus", unixtimeus).
		Str("caller", callerstr).
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

const (
	reqIDUnusedTimeSize    = 3
	reqIDTimeSize          = 5
	reqIDTotalTimeSize     = reqIDUnusedTimeSize + reqIDTimeSize
	reqIDCounterSize       = 3
	reqIDUnusedCounterSize = 1
	reqIDTotalCounterSize  = reqIDCounterSize + reqIDUnusedCounterSize
	reqIDSize              = reqIDTimeSize + reqIDCounterSize
	reqIDCounterMask       = (uint32(1) << (8 * reqIDCounterSize)) - 1
	reqIDCounterShift      = 8 * reqIDUnusedCounterSize
)

var (
	base32RawHexEncoding = base32.HexEncoding.WithPadding(base32.NoPadding)
)

func (s *Server) lreqID(count uint32) string {
	// id looks like:
	// reqIDUnusedTimeSize | reqIDTimeSize | reqIDCounterSize | reqIDUnusedCounterSize
	b := [reqIDTotalTimeSize + reqIDTotalCounterSize]byte{}
	now := uint64(time.Now().Round(0).UnixMilli())
	binary.BigEndian.PutUint64(b[:reqIDTotalTimeSize], now)
	binary.BigEndian.PutUint32(b[reqIDTotalTimeSize:], (count&reqIDCounterMask)<<reqIDCounterShift)
	return s.config.Instance + "-" + base32RawHexEncoding.EncodeToString(b[reqIDUnusedTimeSize:reqIDUnusedTimeSize+reqIDSize])
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
