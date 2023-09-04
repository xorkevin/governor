package gate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGate(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	assert.True(true)
}
