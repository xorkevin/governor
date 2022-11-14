package governor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVCSBuildInfo(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	info := ReadVCSBuildInfo()

	assert.Equal(VCSBuildInfo{
		GoVersion:   info.GoVersion,
		VCSModified: info.VCSModified,
	}, info)

	info.VCS = "git"
	info.VCSRevision = "somehash"
	info.VCSModified = true
	assert.Equal("git-somehash-dev", info.VCSStr())
}
