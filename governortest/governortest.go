package governortest

import (
	"io"

	"xorkevin.dev/governor"
)

func NewTestServer(config, secrets io.Reader) *governor.Server {
	return governor.New(governor.Opts{
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
}
