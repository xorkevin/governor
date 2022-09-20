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
		Version:           opts.Version.String(),
		DisableAutoGenTag: true,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "starts the http server and runs all services",
		Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
		Run: func(cmd *cobra.Command, args []string) {
			c.s.SetFlags(Flags{
				ConfigFile: c.configFile,
			})
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
			c.c.SetFlags(ClientFlags{
				ConfigFile: c.configFile,
			})
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

	rootCmd.PersistentFlags().StringVar(&c.configFile, "config", "", fmt.Sprintf("config file (default is $XDG_CONFIG_HOME/%s/%s.yaml)", opts.Appname, opts.DefaultFile))

	c.cmd = rootCmd
}

// Execute runs the governor cmd
func (c *Cmd) Execute() {
	if err := c.cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
