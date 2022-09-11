package governor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"xorkevin.dev/kerrors"
)

type (
	// LogLevel is a log level
	LogLevel int
)

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelNone
)

// LogLevelFromString creates a log level from a string
func LogLevelFromString(s string) LogLevel {
	switch s {
	case "DEBUG":
		return LogLevelDebug
	case "INFO":
		return LogLevelInfo
	case "WARN":
		return LogLevelWarn
	case "ERROR":
		return LogLevelError
	case "NONE":
		return LogLevelNone
	default:
		return LogLevelInfo
	}
}

// String implements [fmt.Stringer]
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelNone:
		return "NONE"
	default:
		return "UNSET"
	}
}

func logOutputFromString(s string) io.Writer {
	switch s {
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
	// JSONLogWriter writes logs in json format
	JSONLogWriter struct {
		FieldLevel      string
		FieldTime       string
		FieldTimeUnix   string
		FieldTimeUnixUS string
		FieldCaller     string
		FieldPath       string
		FieldMsg        string
		W               io.Writer
	}
)

// NewJSONLogWriter creates a new [*JSONLogWriter]
func NewJSONLogWriter(w io.Writer) *JSONLogWriter {
	return &JSONLogWriter{
		FieldLevel:      "level",
		FieldTime:       "time",
		FieldTimeUnix:   "unixtime",
		FieldTimeUnixUS: "unixtimeus",
		FieldCaller:     "caller",
		FieldPath:       "path",
		FieldMsg:        "msg",
		W:               w,
	}
}

// Log implements [LogWriter]
func (w *JSONLogWriter) Log(level LogLevel, t time.Time, caller *LogFrame, path string, msg string, fields LogFields) {
	timestr := t.Format(time.RFC3339Nano)
	unixtime := t.Unix()
	unixtimeus := t.UnixMicro()
	callerstr := ""
	if caller != nil {
		callerstr = fmt.Sprintf("%s %s:%d", caller.Function, caller.File, caller.Line)
	}
	allFields := LogFields{
		w.FieldLevel:      level,
		w.FieldTime:       timestr,
		w.FieldTimeUnix:   unixtime,
		w.FieldTimeUnixUS: unixtimeus,
		w.FieldCaller:     callerstr,
		w.FieldPath:       path,
		w.FieldMsg:        msg,
	}
	mergeLogFields(allFields, fields)
	b, err := json.Marshal(allFields)
	if err != nil {
		log.Println(err)
		return
	}
	if _, err := w.W.Write(b); err != nil {
		log.Println(err)
	}
}

type (
	// LogFunc returns a log message and fields
	LogFunc = func() (msg string, fields LogFields)

	// Logger writes logs with context
	Logger interface {
		Log(ctx context.Context, level LogLevel, skip int, msg string, fields LogFields)
		LogF(ctx context.Context, level LogLevel, skip int, fn LogFunc)
		Sublogger(path string, fields LogFields) Logger
	}

	// LogWriter is a log service adapter
	LogWriter interface {
		Log(level LogLevel, t time.Time, caller *LogFrame, path string, msg string, fields LogFields)
	}

	// LogFrame is a logger caller frame
	LogFrame struct {
		Function string
		File     string
		Line     int
		PC       uintptr
	}

	// LogFields is associated data
	LogFields map[string]interface{}

	// KLogger is a context logger that writes logs to a [LogWriter]
	KLogger struct {
		minLevel  LogLevel
		logWriter LogWriter
		path      string
		fields    LogFields
		parent    *KLogger
	}

	// LoggerOpt is an options function for [NewLogger]
	LoggerOpt = func(l *KLogger)

	ctxKeyLogFields struct{}

	ctxLogFields struct {
		fields LogFields
		parent *ctxLogFields
	}
)

func getCtxLogFields(ctx context.Context) *ctxLogFields {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(ctxKeyLogFields{})
	if v == nil {
		return nil
	}
	return v.(*ctxLogFields)
}

func setCtxLogFields(ctx context.Context, fields *ctxLogFields) context.Context {
	return context.WithValue(ctx, ctxKeyLogFields{}, fields)
}

func LogWithFields(ctx context.Context, fields LogFields) context.Context {
	return setCtxLogFields(ctx, &ctxLogFields{
		fields: fields,
		parent: getCtxLogFields(ctx),
	})
}

func LogExtendCtx(dest, ctx context.Context, fields LogFields) context.Context {
	return setCtxLogFields(dest, &ctxLogFields{
		fields: fields,
		parent: getCtxLogFields(ctx),
	})
}

// NewLogger creates a new [Logger]
func NewLogger(opts ...LoggerOpt) Logger {
	l := &KLogger{
		minLevel:  LogLevelInfo,
		logWriter: NewJSONLogWriter(NewSyncWriter(os.Stdout)),
		path:      "",
		fields:    nil,
		parent:    nil,
	}
	for _, i := range opts {
		i(l)
	}
	return l
}

func LogOptMinLevel(level LogLevel) LoggerOpt {
	return func(l *KLogger) {
		l.minLevel = level
	}
}

func LogOptMinLevelStr(level string) LoggerOpt {
	return func(l *KLogger) {
		l.minLevel = LogLevelFromString(level)
	}
}

func LogOptWriter(w LogWriter) LoggerOpt {
	return func(l *KLogger) {
		l.logWriter = w
	}
}

func LogOptPath(path string) LoggerOpt {
	return func(l *KLogger) {
		l.path = path
	}
}

func LogOptFields(fields LogFields) LoggerOpt {
	return func(l *KLogger) {
		l.fields = fields
	}
}

func mergeLogFields(dest, from LogFields) {
	for k, v := range from {
		if _, ok := dest[k]; !ok {
			dest[k] = v
		}
	}
}

func (l *KLogger) buildPath(s *strings.Builder) {
	if l.parent != nil {
		l.parent.buildPath(s)
	}
	if l.path != "" {
		s.WriteByte('.')
		s.WriteString(l.path)
	}
}

// Log implements [Logger]
func (l *KLogger) Log(ctx context.Context, level LogLevel, skip int, msg string, fields LogFields) {
	if level < l.minLevel {
		return
	}

	t := time.Now().UTC().Round(0)

	var caller *LogFrame
	callers := [1]uintptr{}
	if n := runtime.Callers(1+skip, callers[:]); n > 0 {
		frame, _ := runtime.CallersFrames(callers[:]).Next()
		caller = &LogFrame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
			PC:       frame.PC,
		}
	}

	allFields := LogFields{}
	mergeLogFields(allFields, fields)
	for f := getCtxLogFields(ctx); f != nil; f = f.parent {
		mergeLogFields(allFields, f.fields)
	}
	for k := l; k != nil; k = k.parent {
		mergeLogFields(allFields, k.fields)
	}
	path := strings.Builder{}
	l.buildPath(&path)
	l.logWriter.Log(level, t, caller, path.String(), msg, allFields)
}

// LogF implements [Logger]
func (l *KLogger) LogF(ctx context.Context, level LogLevel, skip int, fn LogFunc) {
	if level < l.minLevel {
		return
	}

	msg, fields := fn()
	l.Log(ctx, level, 1+skip, msg, fields)
}

// Sublogger creates a new sublogger
func (l *KLogger) Sublogger(path string, fields LogFields) Logger {
	return &KLogger{
		minLevel:  l.minLevel,
		logWriter: l.logWriter,
		path:      path,
		fields:    fields,
		parent:    l,
	}
}

func LogDebug(l Logger, ctx context.Context, msg string, fields LogFields) {
	l.Log(ctx, LogLevelDebug, 1, msg, fields)
}

func LogDebugF(l Logger, ctx context.Context, fn LogFunc) {
	l.LogF(ctx, LogLevelDebug, 1, fn)
}

func LogInfo(l Logger, ctx context.Context, msg string, fields LogFields) {
	l.Log(ctx, LogLevelInfo, 1, msg, fields)
}

func LogInfoF(l Logger, ctx context.Context, fn LogFunc) {
	l.LogF(ctx, LogLevelInfo, 1, fn)
}

func LogWarn(l Logger, ctx context.Context, msg string, fields LogFields) {
	l.Log(ctx, LogLevelWarn, 1, msg, fields)
}

func LogWarnF(l Logger, ctx context.Context, fn LogFunc) {
	l.LogF(ctx, LogLevelWarn, 1, fn)
}

func LogError(l Logger, ctx context.Context, msg string, fields LogFields) {
	l.Log(ctx, LogLevelError, 1, msg, fields)
}

func LogErrorF(l Logger, ctx context.Context, fn LogFunc) {
	l.LogF(ctx, LogLevelError, 1, fn)
}

func LogErr(l Logger, ctx context.Context, err error, fields LogFields) {
	msg := "plain-error"
	var kerr *kerrors.Error
	if errors.As(err, &kerr) {
		msg = kerr.Message
	}
	stacktrace := "NONE"
	var serr *kerrors.StackTrace
	if errors.As(err, &serr) {
		stacktrace = serr.StackString()
	}
	allFields := LogFields{
		"error":      err.Error(),
		"stacktrace": stacktrace,
	}
	mergeLogFields(allFields, fields)
	l.Log(ctx, LogLevelError, 1, msg, fields)
}

func levelToZerologLevel(level LogLevel) zerolog.Level {
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
