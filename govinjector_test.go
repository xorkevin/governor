package governor

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	testInjKeyA struct{}
)

func TestInjector(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	tabReplacer := strings.NewReplacer("\t", "  ")

	server := New(Opts{
		Appname: "govtest",
		Version: Version{
			Num:  "test",
			Hash: "dev",
		},
		Description:  "test gov server",
		EnvPrefix:    "gov",
		ClientPrefix: "govc",
		ConfigReader: strings.NewReader(tabReplacer.Replace(`
http:
	addr: ':8080'
	basepath: /api
setupsecret: setupsecret
`)),
		VaultReader: strings.NewReader(tabReplacer.Replace(`
data:
	setupsecret:
		secret: setupsecret
`)),
		LogWriter: io.Discard,
	})

	pathA := server.Injector()
	pathA.Set(testInjKeyA{}, "abc")

	pathB := server.Injector()
	pathB.Set(testInjKeyA{}, "def")

	pathC := server.Injector()

	assert.Equal(nil, pathC.Get(testInjKeyA{}))
	assert.Equal("abc", pathA.Get(testInjKeyA{}))
	assert.Equal("def", pathB.Get(testInjKeyA{}))
}
