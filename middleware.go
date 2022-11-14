package governor

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	middlewareStripSlashes struct {
		next http.Handler
	}
)

func (m *middlewareStripSlashes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if l := len(path); l > 1 && path[l-1] == '/' {
		r.URL.Path = path[:l-1]
	}
	m.next.ServeHTTP(w, r)
}

func stripSlashesMiddleware(next http.Handler) http.Handler {
	return &middlewareStripSlashes{
		next: next,
	}
}

const (
	headerXForwardedFor = "X-Forwarded-For"
)

type (
	middlewareRealIP struct {
		proxies []netip.Prefix
		next    http.Handler
	}
)

func (m *middlewareRealIP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ip := getRealIP(r, m.proxies)
	if ip != nil {
		ctx = context.WithValue(ctx, ctxKeyRealIP{}, ip)
	}
	m.next.ServeHTTP(w, r.WithContext(ctx))
}

func realIPMiddleware(proxies []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		return &middlewareRealIP{
			proxies: proxies,
			next:    next,
		}
	}
}

func getRealIP(r *http.Request, proxies []netip.Prefix) *netip.Addr {
	host, err := netip.ParseAddrPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return nil
	}
	remoteip := host.Addr()
	if !ipnetsContain(remoteip, proxies) {
		return &remoteip
	}

	xff := r.Header.Get(headerXForwardedFor)
	if xff == "" {
		return &remoteip
	}

	last := remoteip
	ipstrs := strings.Split(xff, ",")
	for i := len(ipstrs) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(ipstrs[i]))
		if err != nil {
			return &remoteip
		}
		if !ipnetsContain(ip, proxies) {
			return &ip
		}
		last = ip
	}

	return &last
}

func ipnetsContain(ip netip.Addr, ipnet []netip.Prefix) bool {
	for _, i := range ipnet {
		if i.Contains(ip) {
			return true
		}
	}
	return false
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

type (
	middlewareReqLogger struct {
		s        *Server
		reqcount *atomic.Uint32
		next     http.Handler
	}
)

func (m *middlewareReqLogger) lreqID() string {
	return m.s.settings.config.Instance + "-" + uid.ReqID(m.reqcount.Add(1))
}

func (m *middlewareReqLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := NewContext(w, r, m.s.log.Logger)
	lreqid := m.lreqID()
	setCtxLocalReqID(c, lreqid)
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
	m.s.log.Info(c.Ctx(), "HTTP request", nil)
	start := time.Now()
	m.next.ServeHTTP(w2, c.Req())
	duration := time.Since(start)
	route := chi.RouteContext(c.Ctx()).RoutePattern()
	if w2.isWS() {
		m.s.log.Info(c.Ctx(), "WS close", klog.Fields{
			"http.ws":          true,
			"http.route":       route,
			"http.status":      w2.status,
			"http.duration_ms": duration.Milliseconds(),
		})
	} else {
		m.s.log.Info(c.Ctx(), "HTTP response", klog.Fields{
			"http.ws":         false,
			"http.route":      route,
			"http.status":     w2.status,
			"http.latency_us": duration.Microseconds(),
		})
	}
}

func (s *Server) reqLoggerMiddleware(next http.Handler) http.Handler {
	return &middlewareReqLogger{
		s:        s,
		reqcount: &atomic.Uint32{},
		next:     next,
	}
}

type (
	middlewareRouteRewrite struct {
		rules []*rewriteRule
		next  http.Handler
	}
)

func (m *middlewareRouteRewrite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, i := range m.rules {
		if i.match(r) {
			r.URL.Path = i.replace(r.URL.Path)
		}
	}
	m.next.ServeHTTP(w, r)
}

func routeRewriteMiddleware(rules []*rewriteRule) Middleware {
	return func(next http.Handler) http.Handler {
		return &middlewareRouteRewrite{
			rules: rules,
			next:  next,
		}
	}
}

type (
	middlewareCorsPathsAllowAll struct {
		rules    []*corsPathRule
		corsNext http.Handler
		next     http.Handler
	}
)

func (m *middlewareCorsPathsAllowAll) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isMatch := false
	for _, i := range m.rules {
		if i.match(r) {
			isMatch = true
			break
		}
	}
	if isMatch {
		m.corsNext.ServeHTTP(w, r)
	} else {
		m.next.ServeHTTP(w, r)
	}
}

func corsPathsAllowAllMiddleware(rules []*corsPathRule) Middleware {
	return func(next http.Handler) http.Handler {
		return &middlewareCorsPathsAllowAll{
			rules:    rules,
			corsNext: cors.AllowAll().Handler(next),
			next:     next,
		}
	}
}

type (
	middlewareBodyLimit struct {
		s     *Server
		limit int64
		next  http.Handler
	}

	maxBytesBodyReader struct {
		body io.ReadCloser
	}
)

// Read implements [io.Reader] on top of a [http.MaxBytesReader]
func (r *maxBytesBodyReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		var rerr *http.MaxBytesError
		if errors.As(err, &rerr) {
			return n, ErrWithRes(err, http.StatusRequestEntityTooLarge, "", "Request too large")
		}
		return n, ErrWithRes(err, http.StatusBadRequest, "", "Failed reading request body")
	}
	return n, err
}

// Close implements [io.Closer] on top of a [http.MaxBytesReader]
func (r *maxBytesBodyReader) Close() error {
	return r.body.Close()
}

func (m *middlewareBodyLimit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ContentLength of -1 is unknown
	if r.ContentLength > m.limit {
		c := NewContext(w, r, m.s.log.Logger)
		c.WriteError(ErrWithRes(nil, http.StatusRequestEntityTooLarge, "", "Request too large"))
		return
	}
	r.Body = &maxBytesBodyReader{
		body: http.MaxBytesReader(w, r.Body, m.limit),
	}
	m.next.ServeHTTP(w, r)
}

func (s *Server) bodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return &middlewareBodyLimit{
			s:     s,
			limit: limit,
			next:  next,
		}
	}
}

const (
	headerAcceptEncoding  = "Accept-Encoding"
	headerContentEncoding = "Content-Encoding"
	headerContentLength   = "Content-Length"
	headerContentType     = "Content-Type"
	headerVary            = "Vary"
)

var (
	defaultCompressibleMediaTypes = []string{
		"application/atom+xml",
		"application/json",
		"application/rss+xml",
		"application/xhtml+xml",
		"application/xml",
		"image/svg+xml",
		"text/css",
		"text/csv",
		"text/html",
		"text/javascript",
		"text/plain",
		"text/xml",
	}
)

const (
	encodingKindZstd = "zstd"
	encodingKindGzip = "gzip"
	encodingKindZlib = "deflate"
)

var (
	defaultPreferredEncodings = []string{
		encodingKindZstd,
		encodingKindGzip,
		encodingKindZlib,
	}
)

type (
	compressWriter interface {
		Kind() string
		io.WriteCloser
		Reset(r io.Writer)
	}

	compressorWriter struct {
		http.ResponseWriter
		r                      *http.Request
		status                 int
		writer                 compressWriter
		compressableMediaTypes map[string]struct{}
		allowedEncodings       map[string]*sync.Pool
		preferredEncodings     []string
		wroteHeader            bool
	}

	identityWriter struct {
		w io.Writer
	}

	pooledZstdWriter struct {
		w *zstd.Encoder
	}

	pooledGzipWriter struct {
		w *gzip.Writer
	}

	pooledZlibWriter struct {
		w *zlib.Writer
	}
)

func (w *compressorWriter) shouldCompress() (string, bool) {
	if w.ResponseWriter.Header().Get(headerContentEncoding) != "" {
		// do not re-compress compressed data
		return "", false
	}
	if w.status == http.StatusSwitchingProtocols {
		// do not compress switched protocols, e.g. websockets
		return "", false
	}
	contentType, _, err := mime.ParseMediaType(w.ResponseWriter.Header().Get(headerContentType))
	if err != nil {
		// invalid media type
		return "", false
	}
	if _, ok := w.compressableMediaTypes[contentType]; !ok {
		// incompressible mimetype
		return "", false
	}
	encodingSet := map[string]struct{}{}
	if accept := strings.TrimSpace(w.r.Header.Get(headerAcceptEncoding)); accept != "" {
		for _, directive := range strings.Split(accept, ",") {
			enc, _, _ := strings.Cut(directive, ";")
			enc = strings.TrimSpace(enc)
			if _, ok := w.allowedEncodings[enc]; ok {
				encodingSet[enc] = struct{}{}
			}
		}
	}
	encoding := ""
	for _, i := range w.preferredEncodings {
		if _, ok := encodingSet[i]; ok {
			encoding = i
			break
		}
	}
	if encoding == "" {
		return "", false
	}
	return encoding, true
}

func (w *compressorWriter) WriteHeader(status int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	if encoding, ok := w.shouldCompress(); ok {
		w.ResponseWriter.Header().Set(headerContentEncoding, encoding)
		w.ResponseWriter.Header().Add(headerVary, headerContentEncoding)
		// compressed length is unknown
		w.ResponseWriter.Header().Del(headerContentLength)
		w.writer = w.allowedEncodings[encoding].Get().(compressWriter)
		w.writer.Reset(w.ResponseWriter)
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressorWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.writer.Write(p)
}

func (w *compressorWriter) Close() error {
	err := w.writer.Close()
	kind := w.writer.Kind()
	if kind == "" {
		return err
	}
	w.allowedEncodings[kind].Put(w.writer)
	return err
}

func (w *identityWriter) Kind() string {
	return ""
}

func (w *identityWriter) Write(p []byte) (int, error) {
	return w.w.Write(p)
}

func (w *identityWriter) Close() error {
	return nil
}

func (w *identityWriter) Reset(r io.Writer) {
	w.w = r
}

func (w *pooledZstdWriter) Kind() string {
	return encodingKindZstd
}

func (w *pooledZstdWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		return n, kerrors.WithMsg(err, "Failed to write to zstd writer")
	}
	return n, nil
}

func (w *pooledZstdWriter) Close() error {
	if err := w.w.Close(); err != nil {
		return kerrors.WithMsg(err, "Failed to close zstd writer")
	}
	return nil
}

func (w *pooledZstdWriter) Reset(wr io.Writer) {
	w.w.Reset(wr)
}

func (w *pooledGzipWriter) Kind() string {
	return encodingKindGzip
}

func (w *pooledGzipWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		return n, kerrors.WithMsg(err, "Failed to write to gzip writer")
	}
	return n, nil
}

func (w *pooledGzipWriter) Close() error {
	if err := w.w.Close(); err != nil {
		return kerrors.WithMsg(err, "Failed to close gzip writer")
	}
	return nil
}

func (w *pooledGzipWriter) Reset(wr io.Writer) {
	w.w.Reset(wr)
}

func (w *pooledZlibWriter) Kind() string {
	return encodingKindZlib
}

func (w *pooledZlibWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		return n, kerrors.WithMsg(err, "Failed to write to zlib writer")
	}
	return n, nil
}

func (w *pooledZlibWriter) Close() error {
	if err := w.w.Close(); err != nil {
		return kerrors.WithMsg(err, "Failed to close zlib writer")
	}
	return nil
}

func (w *pooledZlibWriter) Reset(wr io.Writer) {
	w.w.Reset(wr)
}

type (
	middlewareCompressor struct {
		s                      *Server
		allowedEncodings       map[string]*sync.Pool
		compressableMediaTypes map[string]struct{}
		preferredEncodings     []string
		next                   http.Handler
	}
)

func (m *middlewareCompressor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w2 := &compressorWriter{
		ResponseWriter: w,
		r:              r,
		status:         0,
		writer: &identityWriter{
			w: w,
		},
		compressableMediaTypes: m.compressableMediaTypes,
		allowedEncodings:       m.allowedEncodings,
		preferredEncodings:     m.preferredEncodings,
		wroteHeader:            false,
	}
	defer func() {
		if err := w2.Close(); err != nil {
			m.s.log.Err(r.Context(), kerrors.WithMsg(err, "Failed to close compressor writer"), nil)
		}
	}()
	m.next.ServeHTTP(w2, r)
}

func (s *Server) compressorMiddleware(next http.Handler) http.Handler {
	compressibleTypes := s.settings.middleware.compressibleTypes
	compressableMediaTypes := make(map[string]struct{}, len(compressibleTypes))
	for _, i := range compressibleTypes {
		compressableMediaTypes[i] = struct{}{}
	}
	return &middlewareCompressor{
		s: s,
		allowedEncodings: map[string]*sync.Pool{
			encodingKindZstd: {
				New: func() interface{} {
					w, _ := zstd.NewWriter(nil,
						// 3 is a good tradeoff of size to speed
						zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)),
						zstd.WithEncoderConcurrency(1),
					)
					return &pooledZstdWriter{
						w: w,
					}
				},
			},
			encodingKindGzip: {
				New: func() interface{} {
					// 5 is a good tradeoff of size to speed
					w, _ := gzip.NewWriterLevel(nil, 5)
					return &pooledGzipWriter{
						w: w,
					}
				},
			},
			encodingKindZlib: {
				New: func() interface{} {
					// 5 is a good tradeoff of size to speed
					w, _ := zlib.NewWriterLevel(nil, 5)
					return &pooledZlibWriter{
						w: w,
					}
				},
			},
		},
		compressableMediaTypes: compressableMediaTypes,
		preferredEncodings:     s.settings.middleware.preferredEncodings,
		next:                   next,
	}
}

type (
	middlewareRecoverer struct {
		s    *Server
		next http.Handler
	}
)

func (m *middlewareRecoverer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := NewContext(w, r, m.s.log.Logger)
	defer func() {
		if re := recover(); re != nil {
			if re == http.ErrAbortHandler {
				// may not recover http.ErrAbortHandler so re-panic the error
				panic(re)
			}

			m.s.log.Error(r.Context(), "Panicked in http handler", klog.Fields{
				"recovered":  re,
				"stacktrace": debug.Stack(),
			})

			c.WriteError(ErrWithRes(kerrors.WithMsg(nil, "Panicked in http handler"), http.StatusInternalServerError, "", "Internal Server Error"))
		}
	}()
	m.next.ServeHTTP(c.R())
}

func (s *Server) recovererMiddleware(next http.Handler) http.Handler {
	return &middlewareRecoverer{
		s:    s,
		next: next,
	}
}
