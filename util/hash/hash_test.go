package hash

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Hash(t *testing.T) {
	assert := assert.New(t)
	pass := "password"

	h, err := Hash(pass)
	assert.Nil(err, "hash should be successful")
	assert.Len(h, versionLength+latestConfig.hashLength+latestConfig.saltLength, "hash should be of the proper length")
	assert.True(Verify(pass, h), "password should be correct")

	c, err := newConfig(0)
	assert.Nil(c, "bogus version")
	assert.False(Verify("notpass", h), "incorrect password should fail")
	assert.False(Verify(pass, []byte{}), "passhash of incorrect length should be false")
}
