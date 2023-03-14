package governor

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/term"
	"xorkevin.dev/governor/util/writefs"
	"xorkevin.dev/kerrors"
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
		WFS() writefs.FS
		Log() klog.Logger
	}

	TermConfig struct {
		StdinFd     int
		Stdin       io.Reader
		Stdout      io.Writer
		Stderr      io.Writer
		Fsys        fs.FS
		WFsys       writefs.FS
		Exit        func(code int)
		TermSignals []os.Signal
	}

	termClient struct {
		log     *klog.LevelLogger
		stdinfd int
		stdin   *bufio.Reader
		stdout  io.Writer
		stderr  io.Writer
		fsys    fs.FS
		wfsys   writefs.FS
	}
)

func newTermClient(config *TermConfig, l klog.Logger) Term {
	if config == nil {
		config = &TermConfig{
			StdinFd:     int(os.Stdin.Fd()),
			Stdin:       os.Stdin,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
			Fsys:        os.DirFS("."),
			WFsys:       writefs.NewOSFS("."),
			Exit:        os.Exit,
			TermSignals: []os.Signal{os.Interrupt, syscall.SIGTERM},
		}
	}
	return &termClient{
		log:     klog.NewLevelLogger(l.Sublogger("term")),
		stdinfd: config.StdinFd,
		stdin:   bufio.NewReader(config.Stdin),
		stdout:  config.Stdout,
		stderr:  config.Stderr,
		fsys:    config.Fsys,
		wfsys:   config.WFsys,
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
	if _, err := io.WriteString(c.stdout, "\n"); err != nil {
		return "", kerrors.WithMsg(err, "Failed to write newline")
	}
	return string(s), nil
}

func (c *termClient) FS() fs.FS {
	return c.fsys
}

func (c *termClient) WFS() writefs.FS {
	return c.wfsys
}

func (c *termClient) Log() klog.Logger {
	return c.log.Logger
}

type (
	// Terminal provides convenience terminal functionality
	Terminal struct {
		Term Term
		log  *klog.LevelLogger
	}
)

func NewTerminal(term Term) *Terminal {
	return &Terminal{
		Term: term,
		log:  klog.NewLevelLogger(term.Log()),
	}
}

func (c *Terminal) Stdin() io.Reader {
	return c.Term.Stdin()
}

func (c *Terminal) Stdout() io.Writer {
	return c.Term.Stdout()
}

func (c *Terminal) Stderr() io.Writer {
	return c.Term.Stderr()
}

func (c *Terminal) ReadLine() (string, error) {
	return c.Term.ReadLine()
}

func (c *Terminal) ReadPassword() (string, error) {
	return c.Term.ReadPassword()
}

func (c *Terminal) FS() fs.FS {
	return c.Term.FS()
}

func (c *Terminal) WFS() writefs.FS {
	return c.Term.WFS()
}

func (c *Terminal) Log() klog.Logger {
	return c.log.Logger
}

func (c *Terminal) ReadFile(name string) ([]byte, error) {
	b, err := fs.ReadFile(c.FS(), name)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to read file")
	}
	return b, nil
}

func (c *Terminal) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f, err := c.WFS().OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to open file")
	}
	defer func() {
		if err := f.Close(); err != nil {
			c.log.Err(context.Background(), kerrors.WithMsg(err, "Failed to close open file"))
		}
	}()
	if _, err := f.Write(data); err != nil {
		return kerrors.WithMsg(err, "Failed to write file")
	}
	return nil
}
