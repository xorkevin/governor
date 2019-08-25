package image

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDimensionsFit(t *testing.T) {
	assert := assert.New(t)

	{
		width, height := dimensionsFit(9, 16, 3, 4)
		assert.Equal(2, width)
		assert.Equal(4, height)
	}
	{
		width, height := dimensionsFit(16, 9, 4, 3)
		assert.Equal(4, width)
		assert.Equal(2, height)
	}
	{
		width, height := dimensionsFit(3, 4, 9, 16)
		assert.Equal(9, width)
		assert.Equal(12, height)
	}
	{
		width, height := dimensionsFit(4, 3, 16, 9)
		assert.Equal(12, width)
		assert.Equal(9, height)
	}
}

func TestMaxInt(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(2, maxInt(2, 0))
	assert.Equal(0, maxInt(-1, 0))
}

func TestDimensionsFill(t *testing.T) {
	assert := assert.New(t)

	{
		width, height, offsetX, offsetY := dimensionsFill(9, 16, 3, 4)
		assert.Equal(9, width)
		assert.Equal(12, height)
		assert.Equal(0, offsetX)
		assert.Equal(2, offsetY)
	}
	{
		width, height, offsetX, offsetY := dimensionsFill(16, 9, 4, 3)
		assert.Equal(12, width)
		assert.Equal(9, height)
		assert.Equal(2, offsetX)
		assert.Equal(0, offsetY)
	}
	{
		width, height, offsetX, offsetY := dimensionsFill(3, 4, 9, 16)
		assert.Equal(2, width)
		assert.Equal(4, height)
		assert.Equal(0, offsetX)
		assert.Equal(0, offsetY)
	}
	{
		width, height, offsetX, offsetY := dimensionsFill(4, 3, 16, 9)
		assert.Equal(4, width)
		assert.Equal(2, height)
		assert.Equal(0, offsetX)
		assert.Equal(0, offsetY)
	}
}
