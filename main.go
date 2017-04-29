package governor

import (
	"bytes"
	"database/sql"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
)

type (
	// Error is an error container
	Error struct {
		origin   string
		source   []string
		message  string
		code     int
		status   int
		logLevel int
	}

	responseError struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
)

// NewError creates a new custom Error
func NewError(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelError,
	}
}

// NewErrorWarn creates a new custom Error
func NewErrorWarn(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelWarn,
	}
}

// NewErrorUser creates a new custom Error
func NewErrorUser(origin string, message string, code int, status int) *Error {
	return &Error{
		origin:   origin,
		source:   []string{origin},
		message:  message,
		code:     code,
		status:   status,
		logLevel: levelNoLog,
	}
}

func (e *Error) Error() string {
	return e.Message()
}

// Origin returns the origin of the error message
func (e *Error) Origin() string {
	return e.origin
}

// Source returns the source of the error message
func (e *Error) Source() string {
	k := bytes.NewBufferString(e.source[len(e.source)-1])
	for i := len(e.source) - 2; i > -1; i-- {
		k.WriteString("/")
		k.WriteString(e.source[i])
	}
	return k.String()
}

// Message returns the error message
func (e *Error) Message() string {
	return e.message
}

// Code returns the error code
func (e *Error) Code() int {
	return e.code
}

// Status returns the http status
func (e *Error) Status() int {
	return e.status
}

// Level returns the severity of the error
func (e *Error) Level() int {
	return e.logLevel
}

// AddTrace adds the current caller to the call stack of the error
func (e *Error) AddTrace(module string) {
	e.source = append(e.source, module)
}

func errorHandler(i *echo.Echo, l *logrus.Logger) echo.HTTPErrorHandler {
	return echo.HTTPErrorHandler(func(err error, c echo.Context) {
		if err, ok := err.(*Error); ok {
			switch err.Level() {
			case levelError:
				l.WithFields(logrus.Fields{
					"origin": err.Origin(),
					"source": err.Source(),
					"code":   err.Code(),
				}).Error(err.Message())
			case levelWarn:
				l.WithFields(logrus.Fields{
					"origin": err.Origin(),
					"source": err.Source(),
					"code":   err.Code(),
				}).Warn(err.Message())
			}
			c.JSON(err.Status(), &responseError{
				Message: err.Message(),
				Code:    err.Code(),
			})
		} else {
			l.Warnf("unhandled nongovernor error: %s", err.Error())
			l.Error(err)
			i.DefaultHTTPErrorHandler(err, c)
		}
	})
}

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

type (
	// Config is the server configuration
	Config struct {
		Version     string
		LogLevel    int
		PostgresURL string
	}
)

// NewConfig creates a new server configuration
// It requires ENV vars:
//   VERSION
//   MODE
func NewConfig() Config {
	return Config{
		Version:     os.Getenv("VERSION"),
		LogLevel:    envToLevel(os.Getenv("MODE")),
		PostgresURL: os.Getenv("POSTGRES_URL"),
	}
}

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.LogLevel == levelDebug
}

type (
	// Server is an http gateway
	Server struct {
		i      *echo.Echo
		log    *logrus.Logger
		db     *sql.DB
		config Config
	}
)

// New creates a new Server
func New(config Config) (*Server, error) {
	// logger
	l := logrus.New()
	if config.IsDebug() {
		l.Formatter = &logrus.TextFormatter{}
	} else {
		l.Formatter = &logrus.JSONFormatter{}
	}
	l.Out = os.Stdout
	l.Level = levelToLog(config.LogLevel)

	// http server instance
	i := echo.New()

	// error handling
	i.HTTPErrorHandler = errorHandler(i, l)

	// middleware
	i.Pre(middleware.RemoveTrailingSlash())
	if config.LogLevel == levelDebug {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
	}
	i.Use(middleware.BodyLimit("1M"))
	i.Use(middleware.CORS())
	i.Use(middleware.Recover())
	i.Use(middleware.Gzip())

	// db
	db, err := sql.Open("postgres", config.PostgresURL)
	if err != nil {
		l.Error(err)
		return nil, err
	}
	if err := db.Ping(); err != nil {
		l.Error(err)
		return nil, err
	}

	return &Server{
		i:      i,
		log:    l,
		db:     db,
		config: config,
	}, nil
}

// Start starts the server at the specified port
func (s *Server) Start(port uint) error {
	defer s.db.Close()

	s.i.Logger.Fatal(s.i.Start(":" + strconv.Itoa(int(port))))
	return nil
}

type (
	// Service is an interface for services
	Service interface {
		Mount(c Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error
	}
)

// MountRoute mounts a service
func (s *Server) MountRoute(path string, r Service, m ...echo.MiddlewareFunc) error {
	return r.Mount(s.config, s.i.Group(path, m...), s.db, s.log)
}
