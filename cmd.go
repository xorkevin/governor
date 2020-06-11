package governor

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"os"
	"strings"
)

type (
	Cmd struct {
		s     *Server
		cmd   *cobra.Command
		flags Flags
	}
)

func NewCmd(opts Opts, s *Server) *Cmd {
	c := &Cmd{
		s:     s,
		flags: Flags{},
	}
	c.initCmd(opts)
	return c
}

func (c *Cmd) initCmd(opts Opts) {
	rootCmd := &cobra.Command{
		Use:   opts.Appname,
		Short: opts.Description,
		Long: opts.Description + `

It is built on the governor microservice framework which handles config
management, logging, health checks, setup procedures, jobs, authentication, db,
caching, object storage, emailing, message queues and more.`,
		Version: opts.Version.String(),
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "starts the http server and runs all services",
		Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
		Run: func(cmd *cobra.Command, args []string) {
			c.s.SetFlags(c.flags)
			c.s.Start()
		},
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "runs the setup procedures for all services",
		Long: `Runs the setup procedures for all services

The server first runs all init procedures for all services before running
setup.`,
		Run: func(cmd *cobra.Command, args []string) {
			req, err := getPromptReq()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := req.valid(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := c.s.Setup(*req); err != nil {
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(serveCmd, setupCmd)

	rootCmd.PersistentFlags().StringVar(&c.flags.ConfigFile, "config", "", fmt.Sprintf("config file (default is $XDG_CONFIG_HOME/%s/%s.yaml)", opts.Appname, opts.DefaultFile))

	c.cmd = rootCmd
}

// Execute runs the governor cmd
func (c *Cmd) Execute() {
	if err := c.cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getPromptReq() (*ReqSetup, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("First name: ")
	firstname, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Last name: ")
	lastname, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Password: ")
	passwordBytes, err := terminal.ReadPassword(0)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	password := string(passwordBytes)

	fmt.Print("Verify password: ")
	passwordVerifyBytes, err := terminal.ReadPassword(0)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	passwordVerify := string(passwordVerifyBytes)
	if password != passwordVerify {
		return nil, errors.New("Passwords do not match")
	}

	fmt.Print("Email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	return &ReqSetup{
		Username:  strings.TrimSpace(username),
		Password:  password,
		Email:     strings.TrimSpace(email),
		Firstname: strings.TrimSpace(firstname),
		Lastname:  strings.TrimSpace(lastname),
	}, nil
}
