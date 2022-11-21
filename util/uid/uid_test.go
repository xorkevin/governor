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
		assert.Len(u.Bytes(), 8, "Uid bytes should have the correct length")
		assert.Len(u.Base64(), 11, "Uid base64 should have the correct length")
	}
}

func TestNewSnowflake(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	{
		u, err := NewSnowflake(8)
		assert.NoError(err, "New snowflake should not error")
		assert.NotNil(u, "Snowflake should not be nil")
		assert.Len(u.Bytes(), 8+timeSize, "Snowflake bytes should have the correct length")
		assert.Len(u.Base32(), 26, "Snowflake base32 should have the correct length")
	}
}
