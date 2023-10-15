package governor

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
	"xorkevin.dev/kfs/kfstest"
	"xorkevin.dev/klog"
)

type (
	testServiceC struct {
		log *klog.LevelLogger
	}

	testServiceCReq struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
)

func (s *testServiceC) Register(r ConfigRegistrar) {
}

func (s *testServiceC) Init(ctx context.Context, r ConfigReader, kit ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	m1 := NewMethodRouter(kit.Router)
	m1.AnyCtx("/echo", s.echo)
	m1.AnyCtx("/fail", s.fail)
	return nil
}

func (s *testServiceC) Start(ctx context.Context) error {
	return nil
}

func (s *testServiceC) Stop(ctx context.Context) {
}

func (s *testServiceC) Setup(ctx context.Context, req ReqSetup) error {
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

func (s *testServiceC) fail(c *Context) {
	c.WriteError(ErrWithRes(nil, http.StatusBadRequest, "", "Test fail"))
}

type (
	testClientC struct {
		log         *klog.LevelLogger
		term        Term
		httpc       *HTTPFetcher
		ranRegister bool
		ranInit     bool
	}
)

func (c *testClientC) Register(r ConfigRegistrar, cr CmdRegistrar) {
	c.ranRegister = true

	r.SetDefault("prop1", "val1")

	cr.Register(CmdDesc{
		Usage: "echo",
		Short: "echo input",
		Long:  "test route that echos input",
		Flags: nil,
	}, CmdHandlerFunc(c.echo))
}

func (c *testClientC) Init(r ClientConfigReader, kit ClientKit) error {
	c.ranInit = true

	if u, err := url.Parse(r.Config().BaseURL); err != nil {
		return kerrors.WithMsg(err, "Invalid base url")
	} else if u.Path != "/api" {
		return kerrors.WithMsg(nil, "Mismatched client config")
	}
	if r.Name() != "servicec" {
		return kerrors.WithMsg(nil, "Mismatched client name")
	}
	if r.URL() != "/servicec" {
		return kerrors.WithMsg(nil, "Mismatched client url")
	}
	if !r.GetBool("propbool") {
		return kerrors.WithMsg(nil, "Mismatched prop bool")
	}
	if r.GetInt("propint") != 123 {
		return kerrors.WithMsg(nil, "Mismatched prop int")
	}
	if t, err := r.GetDuration("propdur"); err != nil {
		return kerrors.WithMsg(err, "Mismatched prop int")
	} else if t != 24*time.Hour {
		return kerrors.WithMsg(nil, "Mismatched prop int")
	}
	if r.GetStr("prop1") != "value1" {
		return kerrors.WithMsg(nil, "Mismatched prop str")
	}
	if k := r.GetStrSlice("propslice"); len(k) != 3 || k[0] != "abc" {
		return kerrors.WithMsg(nil, "Mismatched prop str slice")
	}
	var propobj testServiceCReq
	if err := r.Unmarshal("propobj", &propobj); err != nil || propobj != (testServiceCReq{
		Method: "abc",
		Path:   "def",
	}) {
		return kerrors.WithMsg(err, "Mismatched prop obj")
	}

	c.log = klog.NewLevelLogger(kit.Logger)
	c.term = kit.Term
	c.httpc = NewHTTPFetcher(kit.HTTPClient)
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
	if _, err := c.httpc.DoJSON(context.Background(), req, &res); err != nil {
		return err
	}
	b, err := kjson.Marshal(res)
	if err != nil {
		return err
	}
	if _, err := c.term.Stdout().Write(b); err != nil {
		return err
	}

	var buf bytes.Buffer
	f, err := fs.ReadFile(c.term.FS(), "test.txt")
	if err != nil {
		return kerrors.WithMsg(err, "Could not read file")
	}
	if _, err := buf.Write(f); err != nil {
		return err
	}
	ib, err := io.ReadAll(c.term.Stdin())
	if err != nil {
		return kerrors.WithMsg(err, "Could not read stdin")
	}
	if _, err := buf.Write(ib); err != nil {
		return err
	}
	if err := kfs.WriteFile(c.term.FS(), "testoutput.txt", buf.Bytes(), 0o644); err != nil {
		return kerrors.WithMsg(err, "Could not write file")
	}

	return nil
}

func (c *testClientC) echoEmpty(args []string) error {
	req, err := c.httpc.ReqJSON(http.MethodPost, "/echo", testServiceCReq{
		Method: http.MethodPost,
		Path:   "/api/servicec/echo",
	})
	if err != nil {
		return err
	}
	res, err := c.httpc.DoNoContent(context.Background(), req)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(c.term.Stdout(), strconv.Itoa(res.StatusCode)); err != nil {
		return err
	}
	return nil
}

func (c *testClientC) fail(args []string) error {
	req, err := c.httpc.ReqJSON(http.MethodPost, "/fail", testServiceCReq{
		Method: http.MethodPost,
		Path:   "/api/servicec/fail",
	})
	if err != nil {
		return err
	}
	var res testServiceCReq
	if _, err = c.httpc.DoJSON(context.Background(), req, &res); err != nil {
		return err
	}
	return kerrors.WithMsg(nil, "Should have errored")
}

type (
	handlerRoundTripper struct {
		Handler http.Handler
	}
)

func (h handlerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r = r.Clone(klog.ExtendCtx(r.Context(), nil))
	h.Handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func TestClient(t *testing.T) {
	t.Parallel()

	server := New(Opts{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
	}, &ServerOpts{
		ConfigReader: strings.NewReader(`
{
  "http": {
    "addr": ":8080",
    "basepath": "/api"
  }
}
`),
		VaultReader: strings.NewReader(`
{
  "data": {
  }
}
`),
		Fsys: fstest.MapFS{},
	})

	serviceC := &testServiceC{}
	server.Register("servicec", "/servicec", serviceC)

	assert := require.New(t)

	assert.NoError(server.Start(context.Background(), Flags{}, klog.Discard{}))

	var out bytes.Buffer
	fsys := &kfstest.MapFS{
		Fsys: fstest.MapFS{
			"test.txt": &fstest.MapFile{
				Data:    []byte(`test file contents`),
				Mode:    0o644,
				ModTime: time.Now(),
			},
		},
	}

	client := NewClient(Opts{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
	}, &ClientOpts{
		ConfigReader: strings.NewReader(`
{
  "servicec": {
    "propbool": true,
    "propint": 123,
    "propdur": "24h",
    "prop1": "value1",
    "propslice": [
      "abc",
      "def",
      "ghi"
    ],
    "propobj": {
      "method": "abc",
      "path": "def"
    }
  }
}
`),
		HTTPTransport: handlerRoundTripper{
			Handler: server,
		},
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader("test input contents"),
			Stdout:  klog.NewSyncWriter(&out),
			Stderr:  io.Discard,
			Fsys:    fsys,
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

	assert.NoError(client.Init(ClientFlags{}, klog.Discard{}))

	assert.True(clientC.ranRegister)
	assert.True(clientC.ranInit)

	assert.NoError(clientC.echo(nil))
	var echoRes testServiceCReq
	assert.NoError(kjson.Unmarshal(out.Bytes(), &echoRes))
	assert.Equal(testServiceCReq{
		Method: http.MethodPost,
		Path:   "/api/servicec/echo",
	}, echoRes)
	out.Reset()

	outputFile := fsys.Fsys["testoutput.txt"]
	assert.NotNil(outputFile)
	assert.Equal([]byte("test file contentstest input contents"), outputFile.Data)

	assert.NoError(clientC.echoEmpty(nil))
	status, err := strconv.Atoi(out.String())
	assert.NoError(err)
	assert.Equal(200, status)
	out.Reset()

	err = clientC.fail(nil)
	assert.Error(err)
	assert.ErrorIs(err, ErrServerRes)
	var kerr *kerrors.Error
	assert.ErrorAs(err, &kerr)
	assert.Equal("Test fail", kerr.Message)
}
