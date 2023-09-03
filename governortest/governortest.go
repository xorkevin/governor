package governortest

import (
	"context"
	"io"
	"testing"

	"xorkevin.dev/governor"
)

func NewTestServer(t *testing.T, config, secrets io.Reader) *governor.Server {
	t.Helper()
	server := governor.New(governor.Opts{
		Appname: "govtest",
		Version: governor.Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
	}, &governor.ServerOpts{
		ConfigReader: config,
		VaultReader:  secrets,
	})
	t.Cleanup(func() {
		server.Stop(context.Background())
	})
	return server
}
