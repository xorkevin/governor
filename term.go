package governor

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"

	"golang.org/x/term"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/kfs"
	"xorkevin.dev/klog"
)

type (
	Term interface {
		Stdin() io.Reader
		Stdout() io.Writer
		Stderr() io.Writer
		ReadLine() (string, error)
		ReadPassword() (string, error)
		FS() fs.FS
		Log() klog.Logger
	}

	TermConfig struct {
		StdinFd int
		Stdin   io.Reader
		Stdout  io.Writer
		Stderr  io.Writer
		Fsys    fs.FS
	}

	termClient struct {
		log     *klog.LevelLogger
		stdinfd int
		stdin   *bufio.Reader
		stdout  io.Writer
		stderr  io.Writer
		fsys    fs.FS
	}
)

func newTermClient(config *TermConfig, l klog.Logger) Term {
	if config == nil {
		config = &TermConfig{
			StdinFd: int(os.Stdin.Fd()),
			Stdin:   os.Stdin,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
			Fsys:    kfs.DirFS("."),
		}
	}
	return &termClient{
		log:     klog.NewLevelLogger(l.Sublogger("term")),
		stdinfd: config.StdinFd,
		stdin:   bufio.NewReader(config.Stdin),
		stdout:  config.Stdout,
		stderr:  config.Stderr,
		fsys:    config.Fsys,
	}
}

func (c *termClient) Stdin() io.Reader {
	return c.stdin
}

func (c *termClient) Stdout() io.Writer {
	return c.stdout
}

func (c *termClient) Stderr() io.Writer {
	return c.stderr
}

func (c *termClient) ReadLine() (string, error) {
	s, err := c.stdin.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		err = kerrors.WithMsg(err, "Failed to read stdin")
	}
	return s, err
}

func (c *termClient) ReadPassword() (string, error) {
	s, err := term.ReadPassword(c.stdinfd)
	if err != nil {
		return "", kerrors.WithMsg(err, "Failed to read password")
	}
	if _, err := io.WriteString(c.stderr, "\n"); err != nil {
		return "", kerrors.WithMsg(err, "Failed to write newline")
	}
	return string(s), nil
}

func (c *termClient) FS() fs.FS {
	return c.fsys
}

func (c *termClient) Log() klog.Logger {
	return c.log.Logger
}
