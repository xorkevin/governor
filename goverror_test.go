package governor

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
	"testing"
)

func TestNewError(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")

	err := NewError("test message", 123, rootErr)
	assert.NotNil(err, "should not return an empty error")
	k := err.(*goverror)
	assert.Equal("test message", k.message, "error should have the message that was passed in")
	assert.Equal(123, k.status, "error should have the status that was passed in")
	assert.Equal(rootErr, k.err, "error should have the err that was passed in")

	err2 := NewError("", 0, err)
	assert.NotNil(err2, "should not return an empty error")
	k2 := err2.(*goverror)
	assert.Equal("test message", k2.message, "error should have its nearest goverr causer message by default")
	assert.Equal(123, k2.status, "error should have its nearest goverror causer status by default")
	assert.Equal(err, k2.err, "error should have the err that was passed in")

	err3 := NewError("", 0, rootErr)
	assert.NotNil(err2, "should not return an empty error")
	k3 := err3.(*goverror)
	assert.Equal("test root err", k3.message, "error should have its nearest causer message if no goverror by default")
	assert.Equal(0, k3.status, "error should have its nearest goverror causer status if no goverror by default")
	assert.Equal(rootErr, k3.err, "error should have the err that was passed in")

	err4 := NewError("test message", 123, nil)
	assert.NotNil(err4, "should not return an empty error")
	k4 := err4.(*goverror)
	assert.Equal("test message", k4.message, "error should have the message that was passed in")
	assert.Equal(123, k4.status, "error should have the status that was passed in")
	assert.Equal(nil, k4.err, "error should have the err that was passed in")

	usererr := NewErrorUser("test user err", 123, nil)
	assert.NotNil(usererr, "should not return an empty error")
	err5 := NewError("", 0, usererr)
	assert.NotNil(err5, "should not return an empty error")
	k5 := err5.(*goverror)
	assert.Equal("test user err", k5.message, "error should have its nearest causer message by default")
	assert.Equal(123, k5.status, "error should have its nearest causer status by default")
	assert.Equal(usererr, k5.err, "error should have the err that was passed in")
}

func TestNewError_Error(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	err2 := NewError("test message 2", 0, err)
	assert.Equal("test message 2: test message: test root err", err2.Error(), "error string should have the messages of its causers")

	err3 := NewError("test message", 123, nil)
	assert.Equal("test message", err3.Error(), "error string should its message if there are no causers")
}

func TestNewError_Unwrap(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	k := err.(interface {
		Unwrap() error
	})
	assert.Equal(rootErr, k.Unwrap(), "error should return its direct causer when unwrapped")
}

func TestNewError_Is(t *testing.T) {
	assert := assert.New(t)

	err := NewError("test message", 123, nil)
	goverr := &goverror{}
	ok := xerrors.Is(err, goverr)
	assert.True(ok, "error should be a goverror")
}

func TestNewError_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	goverr := &goverror{}
	ok := xerrors.As(err, &goverr)
	assert.True(ok, "error should be a goverror")
	assert.Equal("test message", goverr.message, "error.As should copy message")
	assert.Equal(123, goverr.status, "error.As should copy status")
	assert.Equal(rootErr, goverr.err, "error.As should copy err")
}
