package uid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := New(8)
		assert.NoError(err, "New uid should not error")
		assert.NotNil(u, "Uid should not be nil")
		assert.Len(u.Bytes(), 8, "Uid should have the correct length")
	}
}

func TestUID_FromBase64(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := FromBase64("aGVsbG93b3JsZA")
		assert.NoError(err, "Should not error given a valid base64 encoding")
		assert.Equal([]byte("helloworld"), u.Bytes(), "UID should have the correct bytes representation")
		assert.Equal("aGVsbG93b3JsZA", u.Base64(), "UID should have the correct base64 representation")
	}
	{
		_, err := FromBase64("boguscharacters!@#$%")
		assert.Error(err, "Should error on invalid characters")
	}
}
