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
		log      *klog.LevelLogger
		ranSetup bool
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

func (c *testClientC) Init(r ClientConfigReader, log klog.Logger, term Term, m HTTPClient) error {
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

	c.log = klog.NewLevelLogger(log)
	c.term = term
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

	f, err := fs.ReadFile(c.term.FS(), "test.txt")
	if err != nil {
		return kerrors.WithMsg(err, "Could not read file")
	}
	if err := kfs.WriteFile(c.term.FS(), "testoutput.txt", f, 0o644); err != nil {
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
	_, _, err = c.httpc.DoJSON(context.Background(), req, &res)
	if err != nil {
		return err
	}
	return kerrors.WithMsg(nil, "Should have errored")
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
  },
  "setupsecret": "setupsecret"
}
`),
		VaultReader: strings.NewReader(`
{
  "data": {
    "setupsecret": {
      "secret": "setupsecret"
    }
  }
}
`),
	})

	serviceC := &testServiceC{}
	server.Register("servicec", "/servicec", serviceC)

	assert := require.New(t)

	assert.NoError(server.Init(context.Background(), Flags{}, klog.Discard{}))

	hserver := httptest.NewServer(server)
	t.Cleanup(func() {
		hserver.Close()
		server.Stop(context.Background())
	})

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
		Appname:      "govtest",
		ClientPrefix: "govc",
	}, &ClientOpts{
		ConfigReader: strings.NewReader(`
{
  "http": {
    "baseurl": "` + hserver.URL + `/api"
  },
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
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader("setupsecret"),
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
	out.Reset()

	outputFile := fsys.Fsys["testoutput.txt"]
	assert.NotNil(outputFile)
	assert.Equal([]byte("test file contents"), outputFile.Data)

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
