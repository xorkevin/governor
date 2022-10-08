package governor

import (
	"context"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/go-chi/cors"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

func (s *Server) bodyLimitMiddleware(limit int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ContentLength of -1 is unknown
			if r.ContentLength > limit {
				c := NewContext(w, r, s.log.Logger)
				c.WriteError(ErrWithRes(nil, http.StatusRequestEntityTooLarge, "", "Request too large"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func stripSlashesMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		path := r2.URL.Path
		if l := len(path); l > 1 && path[l-1] == '/' {
			r2.URL.Path = path[:l-1]
		}
		next.ServeHTTP(w, r)
	})
}

func corsPathsAllowAllMiddleware(rules []*corsPathRule) Middleware {
	allowAll := cors.AllowAll()
	return func(next http.Handler) http.Handler {
		corsNext := allowAll.Handler(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isMatch := false
			for _, i := range rules {
				if i.match(r) {
					isMatch = true
					break
				}
			}
			if isMatch {
				corsNext.ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func routeRewriteMiddleware(rules []*rewriteRule) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			for _, i := range rules {
				if i.match(r2) {
					r2.URL.Path = i.replace(r2.URL.Path)
				}
			}
			next.ServeHTTP(w, r2)
		})
	}
}

const (
	headerXForwardedFor = "X-Forwarded-For"
)

type (
	ctxKeyMiddlewareRealIP struct{}
)

func realIPMiddleware(proxies []net.IPNet) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ip := getForwardedForIP(r, proxies)
			if ip != nil {
				ctx = context.WithValue(ctx, ctxKeyMiddlewareRealIP{}, ip)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getCtxMiddlewareRealIP(ctx context.Context) net.IP {
	k := ctx.Value(ctxKeyMiddlewareRealIP{})
	if k == nil {
		return nil
	}
	return k.(net.IP)
}

func getForwardedForIP(r *http.Request, proxies []net.IPNet) net.IP {
	xff := r.Header.Get(headerXForwardedFor)
	if xff == "" {
		return nil
	}

	ipstrs := strings.Split(xff, ",")
	for i := len(ipstrs) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(ipstrs[i]))
		if ip == nil {
			break
		}
		if !ipnetsContain(ip, proxies) {
			return ip
		}
	}

	return nil
}

func ipnetsContain(ip net.IP, ipnet []net.IPNet) bool {
	for _, i := range ipnet {
		if i.Contains(ip) {
			return true
		}
	}
	return false
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

func (s *Server) compressorMiddleware(compressibleTypes []string, preferredEncodings []string) Middleware {
	allowedEncodings := map[string]*sync.Pool{
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
	}
	if len(compressibleTypes) == 0 {
		compressibleTypes = defaultCompressibleMediaTypes
	}
	compressableMediaTypes := make(map[string]struct{}, len(compressibleTypes))
	for _, i := range compressibleTypes {
		compressableMediaTypes[i] = struct{}{}
	}
	if len(preferredEncodings) == 0 {
		preferredEncodings = defaultPreferredEncodings
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w2 := &compressorWriter{
				ResponseWriter: w,
				r:              r,
				status:         0,
				writer: &identityWriter{
					w: w,
				},
				compressableMediaTypes: compressableMediaTypes,
				allowedEncodings:       allowedEncodings,
				preferredEncodings:     preferredEncodings,
				wroteHeader:            false,
			}
			defer func() {
				if err := w2.Close(); err != nil {
					s.log.Err(r.Context(), kerrors.WithMsg(err, "Failed to close compressor writer"), nil)
				}
			}()
			next.ServeHTTP(w2, r)
		})
	}
}

func (s *Server) recovererMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := NewContext(w, r, s.log.Logger)
		defer func() {
			if re := recover(); re != nil {
				if re == http.ErrAbortHandler {
					// may not recover http.ErrAbortHandler so re-panic the error
					panic(re)
				}

				s.log.Error(r.Context(), "Panicked in http handler", klog.Fields{
					"recovered":  re,
					"stacktrace": debug.Stack(),
				})

				c.WriteError(ErrWithRes(kerrors.WithMsg(nil, "Panicked in http handler"), http.StatusInternalServerError, "", "Internal Server Error"))
			}
		}()
		next.ServeHTTP(c.R())
	})
}
