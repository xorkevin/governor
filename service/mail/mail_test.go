package mail

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"
)

func TestBuildMail(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		subject  string
		from     string
		fromName string
		to       []string
		body     string
		htmlbody string
	}{
		{
			"Hello World",
			"kevin@xorkevin.com",
			"Kevin Wang",
			[]string{"other@xorkevin.com", "other2@xorkevin.com"},
			"This is a test plain text alternate that goes over the line limit of 78 characters.",
			"<html><body>This is some test html that goes over the line limit of 78 characters.</body></html>",
		},
	}
	for _, ti := range tests {
		msg, err := msgToBytes(ti.subject, ti.from, ti.fromName, ti.to, []byte(ti.body), []byte(ti.htmlbody))
		assert.NoError(err)
		fmt.Printf(string(msg))
		m, err := mail.ReadMessage(bytes.NewBuffer(msg))
		assert.NoError(err)
		assert.Equal(ti.subject, m.Header.Get("Subject"))
		from, err := m.Header.AddressList("From")
		assert.NoError(err)
		assert.Len(from, 1)
		assert.Equal(ti.from, from[0].Address)
		assert.Equal(ti.fromName, from[0].Name)
		to, err := m.Header.AddressList("To")
		assert.NoError(err)
		assert.Len(to, len(ti.to))
		for n, i := range to {
			assert.Equal(ti.to[n], i.Address)
		}
		assert.Equal(m.Header.Get("Mime-Version"), "1.0")
		mediatype, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
		assert.NoError(err)
		assert.Equal("multipart/mixed", mediatype)
		assert.Equal("UTF-8", params["charset"])
		assert.NotEqual("", params["boundary"])
		multipart.NewReader(m.Body, params["boundary"])
	}
}
