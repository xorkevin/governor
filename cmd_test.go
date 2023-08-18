package governor

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs/kfstest"
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
		term  Term
		flags struct {
			key       string
			unlock    bool
			count     int
			countdown []string
		}
	}
)

func (c *testClientD) Register(inj Injector, r ConfigRegistrar, cr CmdRegistrar) {
	cr.Register(CmdDesc{
		Usage: "echo",
		Short: "echo input",
		Long:  "test route that echos input",
		Flags: []CmdFlag{
			{
				Long:     "key",
				Short:    "i",
				Usage:    "checked value to confirm execution",
				Required: true,
				Value:    &c.flags.key,
				Default:  "bogus",
			},
			{
				Long:     "unlock",
				Short:    "s",
				Usage:    "needed to unlock usage of the command",
				Required: false,
				Value:    &c.flags.unlock,
				Default:  false,
			},
			{
				Long:     "count",
				Short:    "c",
				Usage:    "number of items in the countdown",
				Required: false,
				Value:    &c.flags.count,
				Default:  -1,
			},
			{
				Long:     "countdown",
				Short:    "t",
				Usage:    "countdown array",
				Required: false,
				Value:    &c.flags.countdown,
				Default:  []string{},
			},
		},
	}, CmdHandlerFunc(c.echo))
}

func (c *testClientD) Init(r ClientConfigReader, log klog.Logger, term Term, m HTTPClient) error {
	c.term = term
	return nil
}

func (c *testClientD) echo(args []string) error {
	if !c.flags.unlock {
		return kerrors.WithMsg(nil, "Command not unlocked")
	}
	if c.flags.key != "secret" {
		return kerrors.WithMsg(nil, "Invalid key")
	}
	if c.flags.count < 0 {
		return kerrors.WithMsg(nil, "Invalid count")
	}
	for i, n := c.flags.count, 0; i >= 1; i, n = i-1, n+1 {
		if n >= len(c.flags.countdown) {
			return kerrors.WithMsg(nil, "Missing countdown")
		}
		if c.flags.countdown[n] != strconv.Itoa(i) {
			return kerrors.WithMsg(nil, "Wrong countdown")
		}
	}
	if _, err := io.Copy(c.term.Stderr(), c.term.Stdin()); err != nil {
		return kerrors.WithMsg(err, "Could not copy output")
	}
	return nil
}

func TestCmd(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	var out bytes.Buffer
	stderr := klog.NewSyncWriter(&out)

	client := NewClient(Opts{
		Appname:      "govtest",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader("{}"),
		LogWriter:    io.Discard,
		TermConfig: &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   strings.NewReader("test input content"),
			Stdout:  io.Discard,
			Stderr:  stderr,
			Fsys:    &kfstest.MapFS{Fsys: fstest.MapFS{}},
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
			Stderr:  stderr,
		},
	}, nil, client)

	assert.NoError(cmd.ExecArgs([]string{
		"sd",
		"echo",
		"--key",
		"secret",
		"--unlock",
		"-c",
		"3",
		"-t",
		"3",
		"-t",
		"2",
		"-t",
		"1",
	}))
	assert.Equal("test input content", out.String())
	out.Reset()
}
