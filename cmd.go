package governor

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
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
			if err := s.Setup(); err != nil {
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(serveCmd, setupCmd)

	s.rootCmd = rootCmd
}

// Execute runs the governor cmd
func (s *Server) Execute() {
	if err := s.rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
