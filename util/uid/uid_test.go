package uid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUID(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := New()
		assert.NoError(err, "New uid should not error")
		assert.NotNil(u, "Uid should not be nil")
		assert.Len(u.Bytes(), 16, "Uid bytes should have the correct length")
		assert.Len(u.Base64(), 22, "Uid base64 should have the correct length")
	}
}

func TestSnowflake(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := NewRandSnowflake()
		assert.NoError(err, "New snowflake should not error")
		assert.Len(u.Base64(), 11, "Snowflake base64 should have the correct length")
	}
}

func TestKey(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := NewKey()
		assert.NoError(err, "New key should not error")
		assert.Len(u.Bytes(), 32, "Key bytes should have the correct length")
		assert.Len(u.Base64(), 43, "Key base64 should have the correct length")
	}
}
