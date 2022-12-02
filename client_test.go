package governor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/klog"
)

type (
	testServiceC struct {
		log      *klog.LevelLogger
		ranSetup bool
	}

	testServiceCReq struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
)

func (s *testServiceC) Register(inj Injector, r ConfigRegistrar) {
}

func (s *testServiceC) Init(ctx context.Context, r ConfigReader, l klog.Logger, m Router) error {
	s.log = klog.NewLevelLogger(l)

	m1 := NewMethodRouter(m)
	m1.AnyCtx("/echo", s.echo)
	return nil
}

func (s *testServiceC) Start(ctx context.Context) error {
	return nil
}

func (s *testServiceC) Stop(ctx context.Context) {
}

func (s *testServiceC) Setup(ctx context.Context, req ReqSetup) error {
	s.ranSetup = true
	return nil
}

func (s *testServiceC) Health(ctx context.Context) error {
	return nil
}

func (s *testServiceC) echo(c *Context) {
	c.WriteJSON(http.StatusOK, testServiceCReq{
		Method: c.Req().Method,
		Path:   c.Req().URL.EscapedPath(),
	})
}

func (r testServiceCReq) CloneEmptyPointer() valuer {
	return &testServiceCReq{}
}

func (r *testServiceCReq) Value() interface{} {
	return *r
}

func TestClient(t *testing.T) {
	t.Parallel()

	tabReplacer := strings.NewReplacer("\t", "  ")

	server := New(Opts{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader(tabReplacer.Replace(`
http:
	addr: ':8080'
	basepath: /api
setupsecret: setupsecret
`)),
		VaultReader: strings.NewReader(tabReplacer.Replace(`
data:
	setupsecret:
		secret: setupsecret
`)),
		LogWriter: io.Discard,
	})

	serviceC := &testServiceC{}
	server.Register("servicec", "/servicec", serviceC)

	assert := require.New(t)

	assert.NoError(server.Init(context.Background()))

	hserver := httptest.NewServer(server)
	t.Cleanup(func() {
		hserver.Close()
		server.Stop(context.Background())
	})

	client := NewClient(Opts{
		Appname:      "govtest",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader(tabReplacer.Replace(`
http:
	baseurl: ` + hserver.URL + `/api
`)),
		LogWriter: io.Discard,
	})

	client.SetFlags(ClientFlags{})

	assert.NoError(client.Init())

	setupres, err := client.Setup(context.Background(), "setupsecret")
	assert.NoError(err)
	assert.Equal(&ResSetup{
		Version: "test-dev",
	}, setupres)
	assert.True(serviceC.ranSetup)
}
