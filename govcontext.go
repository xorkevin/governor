package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Context is an http request and writer wrapper
	Context interface {
		LReqID() string
		RealIP() net.IP
		Param(key string) string
		Query(key string) string
		QueryDef(key string, def string) string
		QueryInt(key string, def int) int
		QueryInt64(key string, def int64) int64
		QueryBool(key string) bool
		Header(key string) string
		SetHeader(key, value string)
		AddHeader(key, value string)
		Cookie(key string) (*http.Cookie, error)
		SetCookie(cookie *http.Cookie)
		BasicAuth() (string, string, bool)
		ReadAllBody() ([]byte, error)
		Bind(i interface{}) error
		FormValue(key string) string
		FormFile(key string) (multipart.File, *multipart.FileHeader, error)
		WriteStatus(status int)
		Redirect(status int, url string)
		WriteString(status int, text string)
		WriteJSON(status int, body interface{})
		WriteFile(status int, contentType string, r io.Reader)
		WriteError(err error)
		Ctx() context.Context
		SetCtx(ctx context.Context)
		Get(key interface{}) interface{}
		Set(key, value interface{})
		LogFields(fields klog.Fields)
		Log() klog.Logger
		Req() *http.Request
		Res() http.ResponseWriter
		R() (http.ResponseWriter, *http.Request)
		Websocket() (Websocket, error)
	}

	govcontext struct {
		w        http.ResponseWriter
		r        *http.Request
		query    url.Values
		rawquery string
		log      *klog.LevelLogger
	}
)

// NewContext creates a Context
func NewContext(w http.ResponseWriter, r *http.Request, log klog.Logger) Context {
	return &govcontext{
		w:        w,
		r:        r,
		query:    r.URL.Query(),
		rawquery: r.URL.RawQuery,
		log:      klog.NewLevelLogger(log),
	}
}

func (c *govcontext) LReqID() string {
	return getCtxLocalReqID(c.Ctx())
}

func (c *govcontext) RealIP() net.IP {
	if ip := getCtxMiddlewareRealIP(c.Ctx()); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(c.Req().RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

func (c *govcontext) Param(key string) string {
	return chi.URLParam(c.Req(), key)
}

func (c *govcontext) Query(key string) string {
	if u := c.Req().URL; u.RawQuery != c.rawquery {
		c.query = u.Query()
		c.rawquery = u.RawQuery
	}
	return c.query.Get(key)
}

func (c *govcontext) QueryDef(key string, def string) string {
	v := c.Query(key)
	if v == "" {
		return def
	}
	return v
}

func (c *govcontext) QueryInt(key string, def int) int {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func (c *govcontext) QueryInt64(key string, def int64) int64 {
	s := c.Query(key)
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

func (c *govcontext) QueryBool(key string) bool {
	s := c.Query(key)
	switch s {
	case "t", "true", "y", "yes", "1":
		return true
	default:
		return false
	}
}

func (c *govcontext) Header(key string) string {
	return c.Req().Header.Get(key)
}

func (c *govcontext) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}

func (c *govcontext) AddHeader(key, value string) {
	c.w.Header().Add(key, value)
}

func (c *govcontext) Cookie(key string) (*http.Cookie, error) {
	return c.Req().Cookie(key)
}

func (c *govcontext) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

func (c *govcontext) BasicAuth() (string, string, bool) {
	return c.Req().BasicAuth()
}

func (c *govcontext) ReadAllBody() ([]byte, error) {
	data, err := io.ReadAll(c.Req().Body)
	if err != nil {
		var rerr *http.MaxBytesError
		if errors.As(err, &rerr) {
			return nil, ErrWithRes(err, http.StatusRequestEntityTooLarge, "", "Request too large")
		}
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed reading request")
	}
	return data, nil
}

func (c *govcontext) Bind(i interface{}) error {
	// ContentLength of -1 is unknown
	if c.Req().ContentLength == 0 {
		return ErrWithRes(nil, http.StatusBadRequest, "", "Empty request body")
	}
	mediaType, _, err := mime.ParseMediaType(c.Req().Header.Get("Content-Type"))
	if err != nil {
		return ErrWithRes(err, http.StatusBadRequest, "", "Invalid mime type")
	}
	switch mediaType {
	case "application/json":
		data, err := c.ReadAllBody()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, i); err != nil {
			return ErrWithRes(err, http.StatusBadRequest, "", "Invalid JSON")
		}
	default:
		return ErrWithRes(nil, http.StatusUnsupportedMediaType, "", "Unsupported media type")
	}
	return nil
}

func (c *govcontext) FormValue(key string) string {
	return c.Req().FormValue(key)
}

func (c *govcontext) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.Req().FormFile(key)
}

func (c *govcontext) WriteStatus(status int) {
	c.w.WriteHeader(status)
}

func (c *govcontext) Redirect(status int, url string) {
	http.Redirect(c.Res(), c.Req(), url, status)
}

func (c *govcontext) WriteString(status int, text string) {
	c.w.Header().Set("Content-Type", mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := io.WriteString(c.w, text); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write string bytes"), nil)
	}
}

func (c *govcontext) WriteJSON(status int, body interface{}) {
	b := bytes.Buffer{}
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(body); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write json"), nil)
		http.Error(c.w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	c.w.Header().Set("Content-Type", mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}))
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, &b); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write json bytes"), nil)
	}
}

func (c *govcontext) WriteFile(status int, contentType string, r io.Reader) {
	c.w.Header().Set("Content-Type", contentType)
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, r); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write file"), nil)
		return
	}
}

func (c *govcontext) Ctx() context.Context {
	return c.r.Context()
}

func (c *govcontext) SetCtx(ctx context.Context) {
	c.r = c.r.WithContext(ctx)
}

func (c *govcontext) Get(key interface{}) interface{} {
	return c.Ctx().Value(key)
}

func (c *govcontext) Set(key, value interface{}) {
	c.SetCtx(context.WithValue(c.Ctx(), key, value))
}

func (c *govcontext) LogFields(fields klog.Fields) {
	c.SetCtx(klog.WithFields(c.Ctx(), fields))
}

func (c *govcontext) Log() klog.Logger {
	return c.log.Logger
}

func (c *govcontext) Req() *http.Request {
	return c.r
}

func (c *govcontext) Res() http.ResponseWriter {
	return c.w
}

func (c *govcontext) R() (http.ResponseWriter, *http.Request) {
	return c.Res(), c.Req()
}

const (
	WSProtocolVersion  = "xorkevin.dev-governor_ws_v1alpha1"
	WSReadLimitDefault = 32768
)

type (
	Websocket interface {
		SetReadLimit(limit int64)
		Read(ctx context.Context) (bool, []byte, error)
		Write(ctx context.Context, txt bool, b []byte) error
		Close(status int, reason string)
		CloseError(err error)
	}

	govws struct {
		c    *govcontext
		conn *websocket.Conn
	}
)

func (c *govcontext) Websocket() (Websocket, error) {
	conn, err := websocket.Accept(c.Res(), c.Req(), &websocket.AcceptOptions{
		Subprotocols:    []string{WSProtocolVersion},
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed to open ws connection")
	}
	w := &govws{
		c:    c,
		conn: conn,
	}
	if conn.Subprotocol() != WSProtocolVersion {
		w.Close(int(websocket.StatusPolicyViolation), "Invalid ws subprotocol")
		return nil, ErrWithRes(nil, http.StatusBadRequest, "", "Invalid ws subprotocol")
	}
	w.SetReadLimit(WSReadLimitDefault)
	return w, nil
}

func (w *govws) SetReadLimit(limit int64) {
	w.conn.SetReadLimit(limit)
}

type (
	ErrorWS struct {
		Status int
		Reason string
	}
)

func (e *ErrorWS) Error() string {
	b := strings.Builder{}
	b.WriteString("(")
	b.WriteString(strconv.Itoa(e.Status))
	b.WriteString(")")
	if e.Reason != "" {
		b.WriteString(" ")
		b.WriteString(e.Reason)
	}
	return b.String()
}

func (w *govws) wrapWSErr(err error, status int, reason string) error {
	var werr websocket.CloseError
	if errors.As(err, &werr) {
		return kerrors.WithKind(err, &ErrorWS{
			Status: int(werr.Code),
			Reason: werr.Reason,
		}, "Websocket error")
	}
	return kerrors.WithKind(err, &ErrorWS{
		Status: status,
		Reason: reason,
	}, "Websocket error")
}

// ErrWS returns a wrapped error with a websocket code
func ErrWS(err error, status int, reason string) error {
	return kerrors.WithKind(err, &ErrorWS{
		Status: status,
		Reason: reason,
	}, "Websocket error")
}

func (w *govws) Read(ctx context.Context) (bool, []byte, error) {
	t, b, err := w.conn.Read(ctx)
	if err != nil {
		return false, nil, w.wrapWSErr(err, int(websocket.StatusUnsupportedData), "Failed to read from ws")
	}
	return t == websocket.MessageText, b, nil
}

func (w *govws) Write(ctx context.Context, txt bool, b []byte) error {
	msgtype := websocket.MessageBinary
	if txt {
		msgtype = websocket.MessageText
	}
	reqctx, reqcancel := context.WithTimeout(ctx, 5*time.Second)
	defer reqcancel()
	if err := w.conn.Write(reqctx, msgtype, b); err != nil {
		return w.wrapWSErr(err, int(websocket.StatusInternalError), "Failed to write to ws")
	}
	return nil
}

func (w *govws) Close(status int, reason string) {
	if err := w.conn.Close(websocket.StatusCode(status), reason); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already wrote close") {
			return
		}
		err = w.wrapWSErr(err, int(websocket.StatusInternalError), "Failed to close ws connection")
		w.c.log.WarnErr(w.c.Ctx(), kerrors.WithMsg(err, "Failed to close ws connection"), nil)
	}
}

func (w *govws) CloseError(err error) {
	var werr *ErrorWS
	isError := errors.As(err, &werr)
	if !isError {
		werr = &ErrorWS{
			Status: int(websocket.StatusInternalError),
			Reason: "Internal error",
		}
	}

	if !errors.Is(err, ErrorNoLog{}) {
		if werr.Status != int(websocket.StatusInternalError) {
			w.c.log.WarnErr(w.c.Ctx(), err, nil)
		} else {
			w.c.log.Err(w.c.Ctx(), err, nil)
		}
	}

	w.Close(werr.Status, werr.Reason)
}
