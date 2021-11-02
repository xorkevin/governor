package governor

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
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
management, logging, health checks, setup procedures, jobs, authentication, db,
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

	var setupFirst bool
	var setupSecret string
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "runs the setup procedures for all services",
		Long: `Runs the setup procedures for all services

Calls the server setup endpoint.`,
		Run: func(cmd *cobra.Command, args []string) {
			req := ReqSetup{
				First:  setupFirst,
				Secret: setupSecret,
			}
			if setupFirst {
				var err error
				req.Admin, err = getAdminPromptReq()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
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
	setupCmd.PersistentFlags().BoolVar(&setupFirst, "first", false, "first time setup")
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

func getAdminPromptReq() (*SetupAdmin, error) {
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

	fmt.Print("Email: ")
	email, err := reader.ReadString('\n')
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

	return &SetupAdmin{
		Username:  strings.TrimSpace(username),
		Password:  password,
		Email:     strings.TrimSpace(email),
		Firstname: strings.TrimSpace(firstname),
		Lastname:  strings.TrimSpace(lastname),
	}, nil
}
