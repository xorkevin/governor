package governor

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type (
	// Cmd is the governor cli with both the server and client
	Cmd struct {
		s          *Server
		c          *Client
		cmd        *cobra.Command
		configFile string
	}
)

// NewCmd creates a new Cmd
func NewCmd(opts Opts, s *Server, c *Client) *Cmd {
	cmd := &Cmd{
		s:          s,
		c:          c,
		configFile: "",
	}
	cmd.initCmd(opts)
	return cmd
}

func (c *Cmd) initCmd(opts Opts) {
	rootCmd := &cobra.Command{
		Use:   opts.Appname,
		Short: opts.Description,
		Long: opts.Description + `

It is built on the governor microservice framework which handles config
management, logging, health checks, setup procedures, authentication, db,
caching, object storage, emailing, message queues and more.`,
		Version: opts.Version.String(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
		},
		DisableAutoGenTag: true,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "starts the http server and runs all services",
		Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := c.s.Start(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
		DisableAutoGenTag: true,
	}

	var setupSecret string
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "runs the setup procedures for all services",
		Long: `Runs the setup procedures for all services

Calls the server setup endpoint.`,
		Run: func(cmd *cobra.Command, args []string) {
			req := ReqSetup{
				Secret: setupSecret,
			}
			if err := req.valid(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := c.c.Init(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			res, err := c.c.Setup(req)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("Successfully setup governor:%s\n", res.Version)
		},
		DisableAutoGenTag: true,
	}
	setupCmd.PersistentFlags().StringVar(&setupSecret, "secret", "", "setup secret")

	rootCmd.AddCommand(serveCmd, setupCmd)

	rootCmd.PersistentFlags().StringVar(&c.configFile, "config", "", fmt.Sprintf("config file (default is $XDG_CONFIG_HOME/%s/{%s|%s}.yaml for server and client respectively)", opts.Appname, opts.DefaultFile, opts.ClientDefault))

	c.addTrees(c.c.GetCmds(), rootCmd)

	c.cmd = rootCmd
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
			if err := c.c.Init(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			t.Handler.Handle(c.c.GetConfig(), c.c.GetConfigValueReader(t.ConfigPrefix, t.URLPrefix), args)
		}
	}
	c.addFlags(cmd, t.Desc.Flags)
	c.addTrees(t.Children, cmd)
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
			cmd.MarkFlagRequired(i.Long)
		}
	}
}

// Execute runs the governor cmd
func (c *Cmd) Execute() {
	if err := c.cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
