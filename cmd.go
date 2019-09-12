package governor

import (
	"bufio"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

type (
	govflags struct {
		configFile string
	}
)

func (s *Server) initCommand(conf ConfigOpts) {
	rootCmd := &cobra.Command{
		Use:   conf.Appname,
		Short: conf.Description,
		Long: conf.Description + `

It is built on the governor microservice framework which handles config
management, logging, health checks, setup procedures, jobs, authentication, db,
caching, object storage, emailing, message queues and more.`,
		Version: conf.Version + " " + conf.VersionHash,
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "starts the http server and runs all services",
		Long: `Starts the http server and runs all services

The server first runs all init procedures for all services before starting.`,
		Run: func(cmd *cobra.Command, args []string) {
			s.Start()
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
			if err := s.Setup(*req); err != nil {
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(serveCmd, setupCmd)

	rootCmd.PersistentFlags().StringVar(&s.flags.configFile, "config", "", fmt.Sprintf("config file (default is $XDG_CONFIG_HOME/%s/%s.yaml)", conf.Appname, conf.DefaultFile))

	s.rootCmd = rootCmd
}

// Execute runs the governor cmd
func (s *Server) Execute() {
	if err := s.rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func getPromptReq() (*ReqSetup, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Password: ")
	password, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Print("Email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

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

	fmt.Print("Orgname: ")
	orgname, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	return &ReqSetup{
		Username:  strings.TrimSpace(username),
		Password:  strings.TrimSpace(password),
		Email:     strings.TrimSpace(email),
		Firstname: strings.TrimSpace(firstname),
		Lastname:  strings.TrimSpace(lastname),
		Orgname:   strings.TrimSpace(orgname),
	}, nil
}
