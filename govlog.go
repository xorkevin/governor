package governor

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

const (
	LogLevelDebug = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

func envToLevel(e string) int {
	switch e {
	case "DEBUG":
		return LogLevelDebug
	case "INFO":
		return LogLevelInfo
	case "WARN":
		return LogLevelWarn
	case "ERROR":
		return LogLevelError
	default:
		return LogLevelInfo
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
	// SyncWriter is a thread safe writer
	SyncWriter struct {
		mu *sync.Mutex
		w  io.Writer
	}
)

// NewSyncWriter creates a new [SyncWriter]
func NewSyncWriter(w io.Writer) io.Writer {
	return &SyncWriter{
		mu: &sync.Mutex{},
		w:  w,
	}
}

// Write implements [io.Writer]
func (w *SyncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

type (
	JSONLogWriter struct {
		W io.Writer
	}
)

func NewJSONLogWriter(w io.Writer) *JSONLogWriter {
	return &JSONLogWriter{
		W: w,
	}
}

type (
	// Logger writes logs with context
	Logger interface {
		Debug(ctx context.Context, msg string, labels, annotations map[string]interface{})
		Info(ctx context.Context, msg string, labels, annotations map[string]interface{})
		Warn(ctx context.Context, msg string, labels, annotations map[string]interface{})
		Error(ctx context.Context, msg string, labels, annotations map[string]interface{})
		Err(ctx context.Context, err error, labels, annotations map[string]interface{})
		Sublogger(module string, labels, annotations map[string]interface{}) Logger
	}

	// LogWriter is a log service adapter
	LogWriter interface {
	}

	// LogLabels are searchable features of a log
	LogLabels map[string]interface{}

	// LogAnnotations are non-searchable features of a log
	LogAnnotations map[string]interface{}

	// KLogger is a context logger that writes logs to a [LogWriter]
	KLogger struct {
		MinLevel    int
		LogWriter   LogWriter
		Path        string
		Labels      LogLabels
		Annotations LogAnnotations
	}

	LoggerOpt = func(l *KLogger)
)

func NewLogger(opts ...LoggerOpt) *KLogger {
	l := &KLogger{
		MinLevel:    LogLevelInfo,
		LogWriter:   NewJSONLogWriter(NewSyncWriter(os.Stdout)),
		Path:        "root",
		Labels:      LogLabels{},
		Annotations: LogAnnotations{},
	}
	for _, i := range opts {
		i(l)
	}
	return l
}

func levelToZerologLevel(level int) zerolog.Level {
	switch level {
	case LogLevelDebug:
		return zerolog.DebugLevel
	case LogLevelInfo:
		return zerolog.InfoLevel
	case LogLevelWarn:
		return zerolog.WarnLevel
	case LogLevelError:
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

type (
	zerologTimestampHook struct{}
)

func (h zerologTimestampHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	now := time.Now().UTC().Round(0)
	e.Str("time", now.Format(time.RFC3339))
	e.Int64("unixtime", now.Unix())
	e.Int64("unixms", now.UnixMilli())
}

//func newLogger(c Config) Logger {
//	zerolog.MessageFieldName = "msg"
//	zerolog.TimestampFieldName = "time"
//	zerolog.LevelFieldName = "level"
//	zerolog.CallerFieldName = "caller"
//	w := c.logOutput
//	if c.IsDebug() {
//		w = zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
//			w.Out = c.logOutput
//		})
//	}
//	l := zerolog.New(w).Level(levelToZerologLevel(c.logLevel)).Hook(zerologTimestampHook{})
//	return &govlogger{
//		minLevel: c.logLevel,
//		logger:   &l,
//		module:   "",
//		data:     nil,
//	}
//}
//
//func (l *govlogger) withFields(e *zerolog.Event, msg string, data map[string]string) {
//	if l.module != "" {
//		e.Str("module", l.module)
//	} else {
//		e.Str("module", "root")
//	}
//	for k, v := range l.data {
//		e.Str(k, v)
//	}
//	for k, v := range data {
//		e.Str(k, v)
//	}
//	e.Msg(msg)
//}
//
//func (l *govlogger) Subtree(module string) Logger {
//	m := l.module
//	if m != "" {
//		m += "."
//	}
//	return &govlogger{
//		minLevel: l.minLevel,
//		logger:   l.logger,
//		module:   m + module,
//		data:     l.data,
//	}
//}
//
//func (l *govlogger) WithData(data map[string]string) Logger {
//	nextData := make(map[string]string, len(data)+len(l.data))
//	for k, v := range l.data {
//		nextData[k] = v
//	}
//	for k, v := range data {
//		nextData[k] = v
//	}
//	return &govlogger{
//		minLevel: l.minLevel,
//		logger:   l.logger,
//		module:   l.module,
//		data:     nextData,
//	}
//}
//
//// Debug logs a debug level message
////
//// This message will only be logged when the server configuration is in debug
//// mode.
//func (l *govlogger) Debug(msg string, data map[string]string) {
//	l.withFields(l.logger.Debug(), msg, data)
//}
//
//// Info logs an info level message
//func (l *govlogger) Info(msg string, data map[string]string) {
//	l.withFields(l.logger.Info(), msg, data)
//}
//
//// Warn logs a warning level message
//func (l *govlogger) Warn(msg string, data map[string]string) {
//	l.withFields(l.logger.Warn(), msg, data)
//}
//
//// Error logs a server error level message
//func (l *govlogger) Error(msg string, data map[string]string) {
//	l.withFields(l.logger.Error(), msg, data)
//}
//
//// Fatal logs a fatal error message then exits
//func (l *govlogger) Fatal(msg string, data map[string]string) {
//	l.withFields(l.logger.Fatal(), msg, data)
//}

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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		method := r.Method
		path := r.URL.EscapedPath()
		remote := r.RemoteAddr
		var forwarded string
		if ip := getCtxKeyMiddlewareRealIP(r.Context()); ip != nil {
			forwarded = ip.String()
		}
		if reqIsWS(r) {
			s.logger.Info("WS open", map[string]string{
				"host":      host,
				"method":    method,
				"ws":        "t",
				"path":      path,
				"remote":    remote,
				"forwarded": forwarded,
			})
			start := time.Now()
			next.ServeHTTP(w, r)
			duration := time.Since(start)
			route := chi.RouteContext(r.Context()).RoutePattern()
			s.logger.Info("WS close", map[string]string{
				"host":      host,
				"method":    method,
				"ws":        "f",
				"route":     route,
				"path":      path,
				"remote":    remote,
				"forwarded": forwarded,
				"duration":  duration.String(),
			})
		} else {
			start := time.Now()
			w2 := &govResponseWriter{
				ResponseWriter: w,
				status:         0,
			}
			next.ServeHTTP(w2, r)
			duration := time.Since(start)
			route := chi.RouteContext(r.Context()).RoutePattern()
			s.logger.Info("HTTP response", map[string]string{
				"host":      host,
				"method":    method,
				"ws":        "f",
				"route":     route,
				"path":      path,
				"remote":    remote,
				"forwarded": forwarded,
				"status":    strconv.Itoa(w2.status),
				"latency":   duration.String(),
			})
		}
	})
}
