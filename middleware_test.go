package governor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("compressorWriter", func(t *testing.T) {
		t.Parallel()

		defaultCompressibleMediaTypesSet := make(map[string]struct{}, len(defaultCompressibleMediaTypes))
		for _, i := range defaultCompressibleMediaTypes {
			defaultCompressibleMediaTypesSet[i] = struct{}{}
		}

		for _, tc := range []struct {
			Test       string
			ReqHeaders map[string]string
			ResHeaders map[string]string
			Status     int
			Encoding   string
		}{
			{
				Test: "selects first compression algorithm",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindGzip + ", " + encodingKindZlib,
				},
				ResHeaders: map[string]string{
					headerContentType:   "application/json; charset=utf-8",
					headerContentLength: "123",
				},
				Status:   http.StatusOK,
				Encoding: encodingKindGzip,
			},
			{
				Test: "does not re-compress compressed data",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindGzip + ", " + encodingKindZlib,
				},
				ResHeaders: map[string]string{
					headerContentType:     "application/json; charset=utf-8",
					headerContentEncoding: encodingKindZstd,
				},
				Status:   http.StatusOK,
				Encoding: encodingKindZstd,
			},
			{
				Test: "does not re-compress switched protocol data",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindGzip + ", " + encodingKindZlib,
				},
				ResHeaders: map[string]string{
					headerContentType: "application/json; charset=utf-8",
				},
				Status:   http.StatusSwitchingProtocols,
				Encoding: "",
			},
			{
				Test: "does not compress incompressable content type",
				ReqHeaders: map[string]string{
					headerAcceptEncoding: encodingKindGzip + ", " + encodingKindZlib,
				},
				ResHeaders: map[string]string{
					headerContentType: "image/jpeg",
				},
				Status:   http.StatusOK,
				Encoding: "",
			},
		} {
			t.Run(tc.Test, func(t *testing.T) {
				t.Parallel()

				assert := require.New(t)

				req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
				for k, v := range tc.ReqHeaders {
					req.Header.Set(k, v)
				}
				rec := httptest.NewRecorder()
				w := compressorWriter{
					w:      rec,
					r:      req,
					status: 0,
					writer: &identityWriter{
						w: rec,
					},
					compressableMediaTypes: defaultCompressibleMediaTypesSet,
					allowedEncodings:       defaultAllowedEncodings,
					preferredEncodings:     defaultPreferredEncodings,
					wroteHeader:            false,
				}
				for k, v := range tc.ResHeaders {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.Status)

				assert.Equal(tc.Encoding, rec.Result().Header.Get(headerContentEncoding))
				if tc.Encoding != "" {
					assert.Equal("", rec.Result().Header.Get(headerContentLength))
				}
			})
		}
	})
}
