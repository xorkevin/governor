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
	assert.Equal("test message", k2.message, "error should have its nearest goverror causer message by default")
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

func TestGoverror_Error(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	err2 := NewError("test message 2", 0, err)
	assert.Equal("test message 2: test message: test root err", err2.Error(), "error string should have the messages of its causers")

	err3 := NewError("test message", 123, nil)
	assert.Equal("test message", err3.Error(), "error string should its message if there are no causers")
}

func TestGoverror_Unwrap(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	k := err.(interface {
		Unwrap() error
	})
	assert.Equal(rootErr, k.Unwrap(), "error should return its direct causer when unwrapped")
}

func TestGoverror_Is(t *testing.T) {
	assert := assert.New(t)

	err := NewError("test message", 123, nil)
	goverr := &goverror{}
	ok := xerrors.Is(err, goverr)
	assert.True(ok, "error should be a goverror")
}

func TestGoverror_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewError("test message", 123, rootErr)
	goverr := &goverror{}
	k := err.(interface {
		As(target interface{}) bool
	})
	ok := k.As(goverr)
	assert.True(ok, "error should be a goverror")
	assert.Equal("test message", goverr.message, "error.As should copy message")
	assert.Equal(123, goverr.status, "error.As should copy status")
	assert.Equal(rootErr, goverr.err, "error.As should copy err")
	goverruser := &goverrorUser{}
	ok1 := k.As(goverruser)
	assert.False(ok1, "goverror should not be a goverrorUser by default")

	goverr2 := &goverror{}
	ok2 := xerrors.As(err, &goverr2)
	assert.True(ok2, "error should be a goverror")
	assert.Equal("test message", goverr2.message, "error.As should copy message")
	assert.Equal(123, goverr2.status, "error.As should copy status")
	assert.Equal(rootErr, goverr2.err, "error.As should copy err")
}

func TestNewErrorUser(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")

	err := NewErrorUser("test message", 123, rootErr)
	assert.NotNil(err, "should not return an empty error")
	k := err.(*goverrorUser)
	assert.Equal("test message", k.message, "error should have the message that was passed in")
	assert.Equal(123, k.status, "error should have the status that was passed in")
	assert.Equal(rootErr, k.err, "error should have the err that was passed in")

	err2 := NewErrorUser("", 0, err)
	assert.NotNil(err2, "should not return an empty error")
	k2 := err2.(*goverrorUser)
	assert.Equal("test message", k2.message, "error should have its nearest goverrorUser causer message by default")
	assert.Equal(123, k2.status, "error should have its nearest goverrorUser causer status by default")
	assert.Equal(err, k2.err, "error should have the err that was passed in")

	err3 := NewErrorUser("", 0, rootErr)
	assert.NotNil(err2, "should not return an empty error")
	k3 := err3.(*goverrorUser)
	assert.Equal("test root err", k3.message, "error should have its nearest causer message if no goverrorUser by default")
	assert.Equal(0, k3.status, "error should have its nearest goverrorUser causer status if no goverrorUser by default")
	assert.Equal(rootErr, k3.err, "error should have the err that was passed in")

	err4 := NewErrorUser("test message", 123, nil)
	assert.NotNil(err4, "should not return an empty error")
	k4 := err4.(*goverrorUser)
	assert.Equal("test message", k4.message, "error should have the message that was passed in")
	assert.Equal(123, k4.status, "error should have the status that was passed in")
	assert.Equal(nil, k4.err, "error should have the err that was passed in")

	goverr := NewError("test user err", 123, nil)
	assert.NotNil(goverr, "should not return an empty error")
	err5 := NewErrorUser("", 0, goverr)
	assert.NotNil(err5, "should not return an empty error")
	k5 := err5.(*goverrorUser)
	assert.Equal("test user err", k5.message, "error should have its nearest causer message by default")
	assert.Equal(123, k5.status, "error should have its nearest causer status by default")
	assert.Equal(goverr, k5.err, "error should have the err that was passed in")
}

func TestGoverrorUser_Error(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewErrorUser("test message", 123, rootErr)
	err2 := NewErrorUser("test message 2", 0, err)
	assert.Equal("test message 2: test message: test root err", err2.Error(), "error string should have the messages of its causers")

	err3 := NewErrorUser("test message", 123, nil)
	assert.Equal("test message", err3.Error(), "error string should its message if there are no causers")
}

func TestGoverrorUser_Unwrap(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewErrorUser("test message", 123, rootErr)
	k := err.(interface {
		Unwrap() error
	})
	assert.Equal(rootErr, k.Unwrap(), "error should return its direct causer when unwrapped")
}

func TestGoverrorUser_Is(t *testing.T) {
	assert := assert.New(t)

	err := NewErrorUser("test message", 123, nil)
	goverruser := &goverrorUser{}
	ok := xerrors.Is(err, goverruser)
	assert.True(ok, "error should be a goverrorUser")
}

func TestGoverrorUser_As(t *testing.T) {
	assert := assert.New(t)

	rootErr := errors.New("test root err")
	err := NewErrorUser("test message", 123, rootErr)
	goverruser := &goverrorUser{}
	k := err.(interface {
		As(target interface{}) bool
	})
	ok := k.As(goverruser)
	assert.True(ok, "error should be a goverror")
	assert.Equal("test message", goverruser.message, "error.As should copy message")
	assert.Equal(123, goverruser.status, "error.As should copy status")
	assert.Equal(rootErr, goverruser.err, "error.As should copy err")
	goverr := &goverror{}
	ok1 := k.As(goverr)
	assert.False(ok1, "goverror should not be a goverror by default")

	goverr2 := &goverrorUser{}
	ok2 := xerrors.As(err, &goverr2)
	assert.True(ok2, "error should be a goverror")
	assert.Equal("test message", goverr2.message, "error.As should copy message")
	assert.Equal(123, goverr2.status, "error.As should copy status")
	assert.Equal(rootErr, goverr2.err, "error.As should copy err")
}
