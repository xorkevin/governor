package uid

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Uid(t *testing.T) {
	assert := assert.New(t)
	u, err := New(8, 4, 4, nil)
	assert.NotNil(err, "arg input should be provided")
	assert.Nil(u, "Uid should not be instantiated")
	u, err = New(8, 0, 4, nil)
	assert.Nil(err, "If hashsize is 0, input need not be provided")
	assert.NotNil(u, "Uid should be instantiated")
	assert.Equal(u.size, 12, "Uid byte array should only contain time and random")
	u, err = New(8, 4, 4, []byte{1, 2, 3, 4, 5})
	assert.Nil(err, "All input is valid")
	assert.Equal([]byte{2, 3, 4, 5}, u.Hash(), "Only the last bytes of the input are used for the hash byte array")
	u, err = New(8, 4, 4, []byte{1, 2})
	assert.Equal([]byte{0, 0, 1, 2}, u.Hash(), "The hash byte array should right align the byte input")
}
