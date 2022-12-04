package governor

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor/util/writefs/writefstest"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	testServiceDReq struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	}
)

type (
	testClientD struct {
		term *Terminal
	}
)

func (c *testClientD) Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar) {
	cr.Register(CmdDesc{
		Usage: "echo",
		Short: "echo input",
		Long:  "test route that echos input",
		Flags: nil,
	}, CmdHandlerFunc(c.echo))
}

func (c *testClientD) Init(r ClientConfigReader, log klog.Logger, term Term, m HTTPClient) error {
	c.term = NewTerminal(term)
	return nil
}

func (c *testClientD) echo(args []string) error {
	if _, err := io.Copy(c.term.Stderr(), c.term.Stdin()); err != nil {
		return kerrors.WithMsg(err, "Could not copy output")
	}
	return nil
}

func TestCmd(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	var out bytes.Buffer

	client := NewClient(Opts{
		Appname:      "govtest",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader(""),
		LogWriter:    io.Discard,
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader("test input content"),
			Stdout:  io.Discard,
			Stderr:  klog.NewSyncWriter(&out),
			Fsys:    fstest.MapFS{},
			WFsys:   writefstest.MapFS{},
			Exit:    func(code int) {},
		},
	})

	client.Register("serviced", "/serviced", &CmdDesc{
		Usage: "sd",
		Short: "service d",
		Long:  "interact with service d",
		Flags: nil,
	}, &testClientD{})

	cmd := NewCmd(Opts{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Description: "test gov server",
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader(""),
			Stdout:  io.Discard,
			Stderr:  io.Discard,
			Exit:    func(code int) {},
		},
	}, nil, client)

	assert.NoError(cmd.ExecArgs([]string{"sd", "echo"}))
	assert.Equal("test input content", out.String())
	out.Reset()
}
