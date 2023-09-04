package governortest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"xorkevin.dev/governor"
	"xorkevin.dev/klog"
)

func NewTestServer(t *testing.T, config, secrets io.Reader) *governor.Server {
	t.Helper()

	if config == nil {
		config = strings.NewReader("{}")
	}
	if secrets == nil {
		secrets = strings.NewReader("{}")
	}

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

type (
	emptyReader struct{}
)

func (r emptyReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

type (
	SuccessHandler struct{}
)

func (h SuccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

func NewTestClient(t *testing.T, server http.Handler, config io.Reader, termConfig *governor.TermConfig) *governor.Client {
	t.Helper()

	if config == nil {
		config = strings.NewReader("{}")
	}

	if termConfig == nil {
		termConfig = NewTestTerm()
	}

	if server == nil {
		server = SuccessHandler{}
	}

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

func NewTestTerm() *governor.TermConfig {
	return &governor.TermConfig{
		StdinFd: int(os.Stdin.Fd()),
		Stdin:   emptyReader{},
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Fsys:    fstest.MapFS{},
	}
}
