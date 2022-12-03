package governor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/writefs/writefstest"
	"xorkevin.dev/kerrors"
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

type (
	testClientC struct {
		config      ClientConfigReader
		log         *klog.LevelLogger
		term        *Terminal
		httpc       *HTTPFetcher
		ranRegister bool
		ranInit     bool
	}
)

func (c *testClientC) Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar) {
	c.ranRegister = true

	r.SetDefault("prop1", "val1")
	cr.Register(CmdDesc{}, CmdHandlerFunc(c.echo))
}

func (c *testClientC) Init(r ClientConfigReader, log klog.Logger, term Term, m HTTPClient) error {
	c.ranInit = true
	c.config = r
	c.log = klog.NewLevelLogger(log)
	c.term = NewTerminal(term)
	c.httpc = NewHTTPFetcher(m)
	return nil
}

func (c *testClientC) echo(args []string) error {
	req, err := c.httpc.ReqJSON(http.MethodPost, "/echo", testServiceCReq{
		Method: http.MethodPost,
		Path:   "/api/servicec/echo",
	})
	if err != nil {
		return err
	}
	var res testServiceCReq
	_, decoded, err := c.httpc.DoJSON(context.Background(), req, &res)
	if err != nil {
		return err
	}
	if !decoded {
		return kerrors.WithMsg(nil, "Undecodable response")
	}
	b, err := kjson.Marshal(res)
	if err != nil {
		return err
	}
	if _, err := c.term.Stdout().Write(b); err != nil {
		return err
	}
	return nil
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

	var out bytes.Buffer

	client := NewClient(Opts{
		Appname:      "govtest",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader(tabReplacer.Replace(`
http:
	baseurl: ` + hserver.URL + `/api
`)),
		LogWriter: io.Discard,
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader("setupsecret"),
			Stdout:  klog.NewSyncWriter(&out),
			Stderr:  io.Discard,
			Fsys:    fstest.MapFS{},
			WFsys:   writefstest.MapFS{},
		},
	})

	client.SetFlags(ClientFlags{})

	clientC := &testClientC{}
	client.Register("servicec", "/servicec", &CmdDesc{
		Usage: "sc",
		Short: "service c",
		Long:  "interact with service c",
		Flags: nil,
	}, clientC)

	assert.NoError(client.Init())

	setupres, err := client.Setup(context.Background(), "-")
	assert.NoError(err)
	assert.Equal(&ResSetup{
		Version: "test-dev",
	}, setupres)
	assert.True(serviceC.ranSetup)

	assert.True(clientC.ranRegister)
	assert.True(clientC.ranInit)

	assert.NoError(clientC.echo(nil))
	var echoRes testServiceCReq
	assert.NoError(kjson.Unmarshal(out.Bytes(), &echoRes))
	assert.Equal(testServiceCReq{
		Method: http.MethodPost,
		Path:   "/api/servicec/echo",
	}, echoRes)
}
