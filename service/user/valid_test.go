package user

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidEmail(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Inp   string
		Valid bool
	}{
		{
			Inp:   "gov@xorkevin.com",
			Valid: true,
		},
		{
			Inp:   "gov@xor-kevin.com",
			Valid: true,
		},
		{
			Inp:   "gov@tld",
			Valid: true,
		},
		{
			Inp:   "gov+te.st@tld",
			Valid: true,
		},
		{
			Inp:   "_gov+test-@tld",
			Valid: true,
		},
		{
			Inp:   "+gov@tld",
			Valid: false,
		},
		{
			Inp:   "gov..test@tld",
			Valid: false,
		},
		{
			Inp:   ".gov@tld",
			Valid: false,
		},
		{
			Inp:   "gov.@tld",
			Valid: false,
		},
		{
			Inp:   "gov@-tld",
			Valid: false,
		},
		{
			Inp:   "gov@tld-",
			Valid: false,
		},
		{
			Inp:   "gov@xor..kevin.com",
			Valid: false,
		},
	} {
		tc := tc
		t.Run(tc.Inp, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)
			err := validEmail(tc.Inp)
			if tc.Valid {
				assert.NoError(err)
			} else {
				assert.Error(err)
			}
		})
	}

}
