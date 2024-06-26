package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/netip"
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
	Context struct {
		w   http.ResponseWriter
		r   *http.Request
		log *klog.LevelLogger
	}

	ctxKeyLocalReqID struct{}

	ctxKeyRealIP struct{}
)

// NewContext creates a Context
func NewContext(w http.ResponseWriter, r *http.Request, log klog.Logger) *Context {
	return &Context{
		w:   w,
		r:   r,
		log: klog.NewLevelLogger(log),
	}
}

func getCtxLocalReqID(c *Context) string {
	v := c.Get(ctxKeyLocalReqID{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxLocalReqID(c *Context, lreqid string) {
	c.Set(ctxKeyLocalReqID{}, lreqid)
}

func (c *Context) LReqID() string {
	return getCtxLocalReqID(c)
}

func getCtxMiddlewareRealIP(c *Context) *netip.Addr {
	k := c.Get(ctxKeyRealIP{})
	if k == nil {
		return nil
	}
	return k.(*netip.Addr)
}

func (c *Context) RealIP() *netip.Addr {
	return getCtxMiddlewareRealIP(c)
}

func (c *Context) Param(key string) string {
	return chi.URLParam(c.r, key)
}

func (c *Context) Query(key string) string {
	return c.r.FormValue(key)
}

func (c *Context) QueryDef(key string, def string) string {
	v := c.Query(key)
	if v == "" {
		return def
	}
	return v
}

func (c *Context) QueryInt(key string, def int) int {
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

func (c *Context) QueryInt64(key string, def int64) int64 {
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

func (c *Context) QueryBool(key string) bool {
	s := c.Query(key)
	switch s {
	case "t", "true", "y", "yes", "1":
		return true
	default:
		return false
	}
}

func (c *Context) Header(key string) string {
	return c.r.Header.Get(key)
}

func (c *Context) SetHeader(key, value string) {
	c.w.Header().Set(key, value)
}

func (c *Context) AddHeader(key, value string) {
	c.w.Header().Add(key, value)
}

func (c *Context) DelHeader(key string) {
	c.w.Header().Del(key)
}

func (c *Context) Cookie(key string) (*http.Cookie, error) {
	return c.r.Cookie(key)
}

func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.w, cookie)
}

func (c *Context) BasicAuth() (string, string, bool) {
	return c.r.BasicAuth()
}

func (c *Context) ReadAllBody() ([]byte, error) {
	data, err := io.ReadAll(c.r.Body)
	if err != nil {
		var rerr *ErrorRes
		if errors.As(err, &rerr) {
			return nil, err
		}
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed reading request body")
	}
	return data, nil
}

func (c *Context) Bind(i interface{}, allowUnknown bool) error {
	// ContentLength of -1 is unknown
	if c.r.ContentLength == 0 {
		return ErrWithRes(nil, http.StatusBadRequest, "", "Empty request body")
	}
	mediaType, _, err := mime.ParseMediaType(c.Header(headerContentType))
	if err != nil {
		return ErrWithRes(err, http.StatusBadRequest, "", "No media type")
	}
	switch mediaType {
	case "application/json":
		{
			dec := json.NewDecoder(c.r.Body)
			if !allowUnknown {
				dec.DisallowUnknownFields()
			}
			if err := dec.Decode(i); err != nil {
				var rerr *ErrorRes
				if errors.As(err, &rerr) {
					return err
				}
				// magic error string from encoding/json
				if strings.Contains(strings.ToLower(err.Error()), "json: unknown field") {
					return ErrWithRes(err, http.StatusBadRequest, "", "Unknown field")
				}
				return ErrWithRes(err, http.StatusBadRequest, "", "Invalid JSON")
			}
			if dec.More() {
				return ErrWithRes(nil, http.StatusBadRequest, "", "Invalid JSON")
			}
			return nil
		}
	default:
		return ErrWithRes(nil, http.StatusUnsupportedMediaType, "", "Unsupported media type")
	}
}

func (c *Context) FormValue(key string) string {
	return c.r.PostFormValue(key)
}

func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	file, header, err := c.r.FormFile(key)
	if err != nil {
		var rerr *ErrorRes
		if errors.As(err, &rerr) {
			return nil, nil, err
		}
		return nil, nil, ErrWithRes(err, http.StatusBadRequest, "", "Invalid form file")
	}
	return file, header, nil
}

func (c *Context) WriteStatus(status int) {
	c.w.WriteHeader(status)
}

func (c *Context) Redirect(status int, url string) {
	http.Redirect(c.w, c.r, url, status)
}

func (c *Context) WriteFile(status int, contentType string, r io.Reader) {
	c.SetHeader(headerContentType, contentType)
	c.w.WriteHeader(status)
	if _, err := io.Copy(c.w, r); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write response"))
		return
	}
}

func (c *Context) WriteString(status int, text string) {
	c.WriteFile(status, mime.FormatMediaType("text/plain", map[string]string{"charset": "utf-8"}), strings.NewReader(text))
}

func (c *Context) WriteJSON(status int, body interface{}) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(body); err != nil {
		c.log.Err(c.Ctx(), kerrors.WithMsg(err, "Failed to write json"))
		http.Error(c.w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	c.WriteFile(status, mime.FormatMediaType("application/json", map[string]string{"charset": "utf-8"}), &b)
}

func (c *Context) WriteError(err error) {
	var rerr *ErrorRes
	if !errors.As(err, &rerr) {
		rerr = &ErrorRes{
			Status:  http.StatusInternalServerError,
			Message: "Internal Server Error",
		}
	}

	if !errors.Is(err, ErrNoLog) {
		if rerr.Status >= http.StatusBadRequest && rerr.Status < http.StatusInternalServerError {
			c.log.WarnErr(c.Ctx(), err)
		} else {
			c.log.Err(c.Ctx(), err)
		}
	}

	var tmrErr *ErrorTooManyRequests
	if errors.As(err, &tmrErr) {
		c.SetHeader(retryAfterHeader, tmrErr.RetryAfterTime())
	}

	c.WriteJSON(rerr.Status, rerr)
}

func (c *Context) Ctx() context.Context {
	return c.r.Context()
}

func (c *Context) SetCtx(ctx context.Context) {
	c.r = c.r.WithContext(ctx)
}

func (c *Context) Get(key interface{}) interface{} {
	return c.Ctx().Value(key)
}

func (c *Context) Set(key, value interface{}) {
	c.SetCtx(context.WithValue(c.Ctx(), key, value))
}

func (c *Context) LogAttrs(attrs ...klog.Attr) {
	c.SetCtx(klog.CtxWithAttrs(c.Ctx(), attrs...))
}

func (c *Context) Log() klog.Logger {
	return c.log.Logger
}

func (c *Context) Req() *http.Request {
	return c.r
}

func (c *Context) Res() http.ResponseWriter {
	return c.w
}

func (c *Context) R() (http.ResponseWriter, *http.Request) {
	return c.w, c.r
}

const (
	// WSProtocolVersion is the websocket subprotocol
	//
	// May not contain any separators as defined by [RFC6455 section 4.1].
	//
	// [RFC6455 section 4.1]: https://www.rfc-editor.org/rfc/rfc6455#section-4.1
	WSProtocolVersion  = "xorkevin.dev-governor.ws.v1alpha1"
	WSReadLimitDefault = 32768
)

type (
	// Websocket manages a websocket
	Websocket struct {
		c    *Context
		conn *websocket.Conn
	}
)

func (c *Context) Websocket(protocols []string) (*Websocket, error) {
	conn, err := websocket.Accept(c.w, c.r, &websocket.AcceptOptions{
		Subprotocols:    protocols,
		CompressionMode: websocket.CompressionContextTakeover,
	})
	if err != nil {
		return nil, ErrWithRes(err, http.StatusBadRequest, "", "Failed to open ws connection")
	}
	w := &Websocket{
		c:    c,
		conn: conn,
	}
	w.SetReadLimit(WSReadLimitDefault)
	return w, nil
}

func (w *Websocket) Subprotocol() string {
	return w.conn.Subprotocol()
}

func (w *Websocket) SetReadLimit(limit int64) {
	w.conn.SetReadLimit(limit)
}

type (
	ErrorWS struct {
		Status int
		Reason string
	}
)

func (e *ErrorWS) Error() string {
	var b strings.Builder
	b.WriteString("(")
	b.WriteString(strconv.Itoa(e.Status))
	b.WriteString(")")
	if e.Reason != "" {
		b.WriteString(" ")
		b.WriteString(e.Reason)
	}
	return b.String()
}

func (w *Websocket) wrapWSErr(err error, status int, reason string) error {
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

func (w *Websocket) Read(ctx context.Context) (bool, []byte, error) {
	t, b, err := w.conn.Read(ctx)
	if err != nil {
		return false, nil, w.wrapWSErr(err, int(websocket.StatusUnsupportedData), "Failed to read from ws")
	}
	return t == websocket.MessageText, b, nil
}

func (w *Websocket) Write(ctx context.Context, txt bool, b []byte) error {
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

func (w *Websocket) Close(status int, reason string) {
	if err := w.conn.Close(websocket.StatusCode(status), reason); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "already wrote close") {
			return
		}
		err = w.wrapWSErr(err, int(websocket.StatusInternalError), "Failed to close ws connection")
		w.c.log.WarnErr(w.c.Ctx(), kerrors.WithMsg(err, "Failed to close ws connection"))
	}
}

func (w *Websocket) CloseError(err error) {
	var werr *ErrorWS
	if !errors.As(err, &werr) {
		werr = &ErrorWS{
			Status: int(websocket.StatusInternalError),
			Reason: "Internal error",
		}
	}

	if !errors.Is(err, ErrNoLog) {
		if werr.Status != int(websocket.StatusInternalError) {
			w.c.log.WarnErr(w.c.Ctx(), err)
		} else {
			w.c.log.Err(w.c.Ctx(), err)
		}
	}

	w.Close(werr.Status, werr.Reason)
}
