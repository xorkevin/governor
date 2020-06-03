package mail

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBuildMail(t *testing.T) {
	assert := assert.New(t)

	msg := newMsgBuilder()
	msg.addAddrHeader("From", "Kevin Wang", "kevin@xorkevin.com")
	msg.addHeader("To", "other@xorkevin.com")
	msg.addHeader("Subject", "Hello World")
	msg.addHtmlBody([]byte("<html><body>This is some test html that goes over the line limit of 78 characters.</body></html>"))
	msg.addBody([]byte("This is some test plain text that goes over the line limit of 78 characters."))
	_, err := msg.build()
	assert.NoError(err)
}
