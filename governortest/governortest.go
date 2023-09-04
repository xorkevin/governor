package governortest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"xorkevin.dev/governor"
	"xorkevin.dev/klog"
)

func NewTestServer(t *testing.T, config, secrets io.Reader) *governor.Server {
	t.Helper()
	server := governor.New(governor.Opts{
		Appname: "govtest",
		Version: governor.Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
	}, &governor.ServerOpts{
		ConfigReader: config,
		VaultReader:  secrets,
	})
	t.Cleanup(func() {
		server.Stop(context.Background())
	})
	return server
}

type (
	HandlerRoundTripper struct {
		Handler http.Handler
	}
)

func (h HandlerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r = r.Clone(klog.ExtendCtx(r.Context(), nil))
	h.Handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func NewTestClient(t *testing.T, server http.Handler, config io.Reader, termConfig *governor.TermConfig) *governor.Client {
	t.Helper()

	return governor.NewClient(governor.Opts{
		Appname: "govtest",
		Version: governor.Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
	}, &governor.ClientOpts{
		ConfigReader: config,
		HTTPTransport: HandlerRoundTripper{
			Handler: server,
		},
		TermConfig: termConfig,
	})
}
