package governor

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"xorkevin.dev/governor/util/ksignal"
	"xorkevin.dev/klog"
)

type (
	// Cmd is the governor cli with both the server and client
	Cmd struct {
		s        *Server
		c        *Client
		cmd      *cobra.Command
		log      *klog.LevelLogger
		opts     Opts
		cmdOpts  CmdOpts
		cmdFlags cmdTopLevelFlags
	}

	cmdTopLevelFlags struct {
		configFile   string
		clientConfig string
		logLevel     string
		logPlain     bool
		docOutputDir string
	}

	// CmdOpts are cmd options
	CmdOpts struct {
		LogWriter io.Writer
	}
)

// NewCmd creates a new Cmd
func NewCmd(opts Opts, cmdOpts *CmdOpts, s *Server, c *Client) *Cmd {
	if cmdOpts == nil {
		cmdOpts = &CmdOpts{}
	}
	cmd := &Cmd{
		s:       s,
		c:       c,
		opts:    opts,
		cmdOpts: *cmdOpts,
	}
	cmd.initCmd()
	return cmd
}

func (c *Cmd) initCmd() {
	rootCmd := &cobra.Command{
		Use:               c.opts.Appname,
		Short:             c.opts.Description,
		Long:              c.opts.Description,
		Version:           c.opts.Version.String(),
		PersistentPreRun:  c.prerun,
		DisableAutoGenTag: true,
	}
	rootCmd.PersistentFlags().StringVar(&c.cmdFlags.configFile, "config", "", fmt.Sprintf("config file (default is %s.json)", c.opts.DefaultFile))
	rootCmd.PersistentFlags().StringVar(&c.cmdFlags.clientConfig, "client-config", "", fmt.Sprintf("client config file (default is %s.json)", c.opts.ClientDefault))
	rootCmd.PersistentFlags().StringVar(&c.cmdFlags.logLevel, "log-level", "info", "log level")
	rootCmd.PersistentFlags().BoolVar(&c.cmdFlags.logPlain, "log-plain", false, "output plain text logs")

	if c.s != nil {
		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "starts the http server and runs all services",
			Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
			Run:               c.execServe,
			DisableAutoGenTag: true,
		}
		rootCmd.AddCommand(serveCmd)
	}

	if c.c != nil {
		setupCmd := &cobra.Command{
			Use:               "setup",
			Short:             "runs the setup procedures for all services",
			Long:              `Runs the setup procedures for all services`,
			Run:               c.execSetup,
			DisableAutoGenTag: true,
		}
		rootCmd.AddCommand(setupCmd)
	}

	docCmd := &cobra.Command{
		Use:               "doc",
		Short:             "generate documentation",
		Long:              `Generate documentation in several formats`,
		DisableAutoGenTag: true,
	}
	docCmd.PersistentFlags().StringVarP(&c.cmdFlags.docOutputDir, "output", "o", ".", "documentation output path")
	rootCmd.AddCommand(docCmd)

	docManCmd := &cobra.Command{
		Use:               "man",
		Short:             "generate man page documentation",
		Long:              `Generate man page documentation`,
		Run:               c.docMan,
		DisableAutoGenTag: true,
	}
	docCmd.AddCommand(docManCmd)

	docMdCmd := &cobra.Command{
		Use:               "md",
		Short:             "generate markdown documentation",
		Long:              `Generate markdown documentation`,
		Run:               c.docMd,
		DisableAutoGenTag: true,
	}
	docCmd.AddCommand(docMdCmd)

	if c.c != nil {
		c.addTrees(c.c.GetCmds(), rootCmd)
	}

	c.cmd = rootCmd
}

func (c *Cmd) logFatal(err error) {
	c.log.Err(context.Background(), err)
	os.Exit(1)
}

func (c *Cmd) runInt(f func(ctx context.Context) error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	var ferr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		ferr = f(ctx)
	}()

	ksignal.Wait(ctx, os.Interrupt, syscall.SIGTERM)

	cancel()
	wg.Wait()

	if ferr != nil {
		c.logFatal(ferr)
		return
	}
}

func (c *Cmd) prerun(cmd *cobra.Command, args []string) {
	logWriter := c.cmdOpts.LogWriter
	if logWriter == nil {
		logWriter = os.Stderr
	}
	logWriter = klog.NewSyncWriter(logWriter)
	var handler *klog.SlogHandler
	if c.cmdFlags.logPlain {
		handler = klog.NewTextSlogHandler(logWriter)
		handler.FieldTimeInfo = ""
		handler.FieldCaller = ""
		handler.FieldMod = ""
	} else {
		handler = klog.NewJSONSlogHandler(logWriter)
	}
	c.log = klog.NewLevelLogger(klog.New(
		klog.OptHandler(handler),
		klog.OptMinLevelStr(c.cmdFlags.logLevel),
	))
}

func (c *Cmd) serve(ctx context.Context) error {
	return c.s.Serve(ctx, Flags{
		ConfigFile: c.cmdFlags.configFile,
		LogPlain:   c.cmdFlags.logPlain,
	}, c.log.Logger)
}

func (c *Cmd) execServe(cmd *cobra.Command, args []string) {
	c.runInt(c.serve)
}

func (c *Cmd) setup(ctx context.Context) error {
	return c.s.Setup(ctx, Flags{
		ConfigFile: c.cmdFlags.configFile,
		LogPlain:   c.cmdFlags.logPlain,
	}, c.log.Logger)
}

func (c *Cmd) execSetup(cmd *cobra.Command, args []string) {
	c.runInt(c.setup)
}

func (c *Cmd) clientInit() {
	if err := c.c.Init(ClientFlags{
		ConfigFile: c.cmdFlags.clientConfig,
	}, c.log.Logger); err != nil {
		c.logFatal(err)
		return
	}
}

func (c *Cmd) docMan(cmd *cobra.Command, args []string) {
	if err := doc.GenManTree(c.cmd, &doc.GenManHeader{
		Title:   c.opts.Appname,
		Section: "1",
	}, c.cmdFlags.docOutputDir); err != nil {
		c.logFatal(err)
		return
	}
}

func (c *Cmd) docMd(cmd *cobra.Command, args []string) {
	if err := doc.GenMarkdownTree(c.cmd, c.cmdFlags.docOutputDir); err != nil {
		c.logFatal(err)
		return
	}
}

func (c *Cmd) addTrees(t []*CmdTree, parent *cobra.Command) {
	for _, i := range t {
		c.addTree(i, parent)
	}
}

func (c *Cmd) addTree(t *CmdTree, parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:               t.Desc.Usage,
		Short:             t.Desc.Short,
		Long:              t.Desc.Long,
		DisableAutoGenTag: true,
	}
	if t.Handler != nil {
		cmd.Run = func(cmd *cobra.Command, args []string) {
			c.clientInit()
			if err := t.Handler.Handle(args); err != nil {
				c.logFatal(err)
				return
			}
		}
	}
	c.addFlags(cmd, t.Desc.Flags)
	c.addTrees(t.Children, cmd)
	parent.AddCommand(cmd)
}

func (c *Cmd) addFlags(cmd *cobra.Command, flags []CmdFlag) {
	for _, i := range flags {
		if i.Value == nil {
			panic("governor: client flag value may not be nil")
		}
		switch v := i.Value.(type) {
		case *bool:
			{
				var dv bool
				if i.Default != nil {
					var ok bool
					dv, ok = i.Default.(bool)
					if !ok {
						panic("governor: client must have same default value type as value")
					}
				}
				cmd.PersistentFlags().BoolVarP(v, i.Long, i.Short, dv, i.Usage)
			}
		case *int:
			{
				var dv int
				if i.Default != nil {
					var ok bool
					dv, ok = i.Default.(int)
					if !ok {
						panic("governor: client must have same default value type as value")
					}
				}
				cmd.PersistentFlags().IntVarP(v, i.Long, i.Short, dv, i.Usage)
			}
		case *string:
			{
				var dv string
				if i.Default != nil {
					var ok bool
					dv, ok = i.Default.(string)
					if !ok {
						panic("governor: client must have same default value type as value")
					}
				}
				cmd.PersistentFlags().StringVarP(v, i.Long, i.Short, dv, i.Usage)
			}
		case *[]string:
			{
				var dv []string
				if i.Default != nil {
					var ok bool
					dv, ok = i.Default.([]string)
					if !ok {
						panic("governor: client must have same default value type as value")
					}
				}
				cmd.PersistentFlags().StringArrayVarP(v, i.Long, i.Short, dv, i.Usage)
			}
		default:
			panic("governor: invalid client flag value type")
		}
		if i.Required {
			cmd.MarkPersistentFlagRequired(i.Long)
		}
	}
}

// ExecArgs runs the command with the provided arguments
func (c *Cmd) ExecArgs(args []string, termConfig *TermConfig) error {
	c.cmd.SetArgs(args)
	if termConfig != nil {
		c.cmd.SetIn(termConfig.Stdin)
		c.cmd.SetOut(termConfig.Stdout)
		c.cmd.SetErr(termConfig.Stderr)
	}
	if err := c.cmd.Execute(); err != nil {
		return err
	}
	return nil
}

// Execute runs the governor cmd
func (c *Cmd) Execute() {
	if err := c.ExecArgs(os.Args[1:], nil); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
		return
	}
}
