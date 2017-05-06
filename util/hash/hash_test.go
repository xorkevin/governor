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
	c, err := newConfig(vLatest)
	assert.Nil(err, "there should be no error when creating the latest config")
	assert.Equal(latestConfig, c, "newConfig should produce the latest config provided the version vLatest")
	assert.Equal(vLatest, c.Version(), "the version numbers should match")
}
