package governor

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"xorkevin.dev/governor/util/ksignal"
)

type (
	// Cmd is the governor cli with both the server and client
	Cmd struct {
		s          *Server
		c          *Client
		cmd        *cobra.Command
		opts       Opts
		configFile string
		cmdFlags   cmdTopLevelFlags
	}

	cmdTopLevelFlags struct {
		setupSecret  string
		docOutputDir string
	}
)

// NewCmd creates a new Cmd
func NewCmd(opts Opts, s *Server, c *Client) *Cmd {
	cmd := &Cmd{
		s:          s,
		c:          c,
		opts:       opts,
		configFile: "",
	}
	cmd.initCmd()
	return cmd
}

func (c *Cmd) initCmd() {
	rootCmd := &cobra.Command{
		Use:   c.opts.Appname,
		Short: c.opts.Description,
		Long: c.opts.Description + `

It is built on the governor microservice framework which handles config
management, logging, health checks, setup procedures, authentication, db,
caching, object storage, emailing, message queues and more.`,
		Version:           c.opts.Version.String(),
		PersistentPreRun:  c.prerun,
		DisableAutoGenTag: true,
	}
	rootCmd.PersistentFlags().StringVar(&c.configFile, "config", "", fmt.Sprintf("config file (default is $XDG_CONFIG_HOME/%s/{%s|%s}.json for server and client respectively)", c.opts.Appname, c.opts.DefaultFile, c.opts.ClientDefault))

	if c.s != nil {
		serveCmd := &cobra.Command{
			Use:   "serve",
			Short: "starts the http server and runs all services",
			Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
			Run:               c.serve,
			DisableAutoGenTag: true,
		}
		rootCmd.AddCommand(serveCmd)
	}

	if c.c != nil {
		setupCmd := &cobra.Command{
			Use:   "setup",
			Short: "runs the setup procedures for all services",
			Long: `Runs the setup procedures for all services

Calls the server setup endpoint.`,
			Run:               c.setup,
			DisableAutoGenTag: true,
		}
		setupCmd.PersistentFlags().StringVar(&c.cmdFlags.setupSecret, "secret", "", "setup secret")
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

func (c *Cmd) logFatal(v interface{}) {
	var errout io.Writer = os.Stderr
	exit := os.Exit
	if conf := c.opts.TermConfig; conf != nil {
		errout = conf.Stderr
		exit = conf.Exit
	}
	fmt.Fprintln(errout, v)
	exit(1)
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

	var signals []os.Signal
	if conf := c.opts.TermConfig; conf != nil {
		signals = conf.TermSignals
	}
	ksignal.Wait(ctx, signals...)

	cancel()
	wg.Wait()

	if ferr != nil {
		c.logFatal(ferr)
	}
}

func (c *Cmd) prerun(cmd *cobra.Command, args []string) {
	if c.s != nil {
		c.s.SetFlags(Flags{
			ConfigFile: c.configFile,
		})
	}
	if c.c != nil {
		c.c.SetFlags(ClientFlags{
			ConfigFile: c.configFile,
		})
	}
}

func (c *Cmd) serve(cmd *cobra.Command, args []string) {
	c.runInt(c.s.Serve)
}

func (c *Cmd) setup(cmd *cobra.Command, args []string) {
	if err := c.c.Init(); err != nil {
		c.logFatal(err)
		return
	}
	if _, err := c.c.Setup(context.Background(), c.cmdFlags.setupSecret); err != nil {
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

func (c *Cmd) addTrees(t []*cmdTree, parent *cobra.Command) {
	for _, i := range t {
		c.addTree(i, parent)
	}
}

func (c *Cmd) addTree(t *cmdTree, parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:               t.Desc.Usage,
		Short:             t.Desc.Short,
		Long:              t.Desc.Long,
		DisableAutoGenTag: true,
	}
	if t.Handler != nil {
		cmd.Run = func(cmd *cobra.Command, args []string) {
			if err := c.c.Init(); err != nil {
				c.logFatal(err)
				return
			}
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
func (c *Cmd) ExecArgs(args []string) error {
	c.cmd.SetArgs(args)
	if conf := c.opts.TermConfig; conf != nil {
		c.cmd.SetIn(conf.Stdin)
		c.cmd.SetOut(conf.Stdout)
		c.cmd.SetErr(conf.Stderr)
	}
	if err := c.cmd.Execute(); err != nil {
		return err
	}
	return nil
}

// Execute runs the governor cmd
func (c *Cmd) Execute() {
	if err := c.ExecArgs(os.Args[1:]); err != nil {
		c.logFatal(err)
		return
	}
}
