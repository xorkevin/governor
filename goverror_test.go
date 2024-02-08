package governor

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestError(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	for _, tc := range []struct {
		Err    error
		String string
	}{
		{
			Err:    ErrNoLog,
			String: "No log",
		},
		{
			Err:    ErrUnreachable,
			String: "Unreachable code. Invariant violated",
		},
		{
			Err: &ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    "test_code",
				Message: "test error message",
			},
			String: "(400) test error message [test_code]",
		},
		{
			Err: &ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    "test_code",
				Message: "test error message",
			},
			String: "(400) test error message [test_code]",
		},
		{
			Err: &ErrorTooManyRequests{
				RetryAfter: time.Date(1991, time.August, 25, 20, 57, 9, 0, time.UTC),
			},
			String: "Too many requests. Try again after Sun, 25 Aug 1991 20:57:09 UTC.",
		},
		{
			Err:    ErrInvalidConfig,
			String: "Invalid config",
		},
		{
			Err:    ErrVault,
			String: "Failed vault request",
		},
		{
			Err:    ErrInvalidClientReq,
			String: "Invalid client request",
		},
		{
			Err:    ErrSendClientReq,
			String: "Failed sending client request",
		},
		{
			Err:    ErrInvalidServerRes,
			String: "Invalid server response",
		},
	} {
		assert.Equal(tc.String, tc.Err.Error())
	}

	for _, tc := range []struct {
		Err    error
		String string
	}{
		{
			Err:    ErrWithNoLog(nil),
			String: "No log",
		},
		{
			Err:    ErrWithRes(nil, http.StatusUnauthorized, "another_test_code", "yet another test error message"),
			String: "(401) yet another test error message [another_test_code]",
		},
		{
			Err:    ErrWithUnreachable(nil, "test unreachable code"),
			String: "Unreachable code. Invariant violated",
		},
		{
			Err:    ErrWithTooManyRequests(nil, time.Date(1991, time.August, 25, 20, 57, 9, 0, time.UTC), "too_many_test_code", "too many test requests"),
			String: "Too many requests. Try again after Sun, 25 Aug 1991 20:57:09 UTC.",
		},
	} {
		assert.Contains(tc.Err.Error(), tc.String)
	}
}
