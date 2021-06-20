package image

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDimensionsFit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		FromW int
		FromH int
		ToW   int
		ToH   int
		ExpW  int
		ExpH  int
	}{
		{
			Test:  "shrink height constrained",
			FromW: 9,
			FromH: 16,
			ToW:   3,
			ToH:   4,
			ExpW:  2,
			ExpH:  4,
		},
		{
			Test:  "shrink width constrained",
			FromW: 16,
			FromH: 9,
			ToW:   4,
			ToH:   3,
			ExpW:  4,
			ExpH:  2,
		},
		{
			Test:  "grow width constrained",
			FromW: 3,
			FromH: 4,
			ToW:   9,
			ToH:   16,
			ExpW:  9,
			ExpH:  12,
		},
		{
			Test:  "grow height constrained",
			FromW: 4,
			FromH: 3,
			ToW:   16,
			ToH:   9,
			ExpW:  12,
			ExpH:  9,
		},
	} {
		t.Run(tc.Test, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)

			width, height := dimensionsFit(tc.FromW, tc.FromH, tc.ToW, tc.ToH)
			assert.Equal(tc.ExpW, width)
			assert.Equal(tc.ExpH, height)
		})
	}
}

func TestMaxInt(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	assert.Equal(2, maxInt(2, 0))
	assert.Equal(0, maxInt(-1, 0))
}

func TestDimensionsFill(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		FromW int
		FromH int
		ToW   int
		ToH   int
		ExpW  int
		ExpH  int
		ExpX  int
		ExpY  int
	}{
		{
			Test:  "shrink width constrained",
			FromW: 9,
			FromH: 16,
			ToW:   3,
			ToH:   4,
			ExpW:  9,
			ExpH:  12,
			ExpX:  0,
			ExpY:  2,
		},
		{
			Test:  "shrink height constrained",
			FromW: 16,
			FromH: 9,
			ToW:   4,
			ToH:   3,
			ExpW:  12,
			ExpH:  9,
			ExpX:  2,
			ExpY:  0,
		},
		{
			Test:  "grow height constrained",
			FromW: 3,
			FromH: 4,
			ToW:   9,
			ToH:   16,
			ExpW:  2,
			ExpH:  4,
			ExpX:  0,
			ExpY:  0,
		},
		{
			Test:  "grow width constrained",
			FromW: 4,
			FromH: 3,
			ToW:   16,
			ToH:   9,
			ExpW:  4,
			ExpH:  2,
			ExpX:  0,
			ExpY:  0,
		},
	} {
		t.Run(tc.Test, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)

			width, height, offsetX, offsetY := dimensionsFill(tc.FromW, tc.FromH, tc.ToW, tc.ToH)
			assert.Equal(tc.ExpW, width)
			assert.Equal(tc.ExpH, height)
			assert.Equal(tc.ExpX, offsetX)
			assert.Equal(tc.ExpY, offsetY)
		})
	}
}
