package hash

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Uid(t *testing.T) {
	assert := assert.New(t)
	pass := "password"

	h, err := Hash(pass)
	assert.Nil(err, "hash should be successful")
	assert.Len(h, versionLength+latestConfig.hashLength+latestConfig.saltLength, "hash should be of the proper length")
	assert.True(Verify(pass, h), "password should be correct")

	hs, err := Strong(pass)
	assert.Nil(err, "strong hash should be successful")
	assert.True(Verify(pass, hs), "strong hash password should be correct")

	hf, err := Fast(pass)
	assert.Nil(err, "fast hash should be successful")
	assert.True(Verify(pass, hf), "fast hash password should be correct")

	c, err := newConfig(latestConfig.version)
	assert.Nil(err, "there should be no error when creating the latest config")
	assert.Equal(latestConfig, c, "newConfig should produce the latest config provided the version vLatest")
	assert.Equal(latestConfig.version, c.Version(), "the version numbers should match")
	assert.False(Verify("notpass", h), "incorrect password should fail")
	assert.False(Verify(pass, []byte{}), "passhash of incorrect length should be false")
}
