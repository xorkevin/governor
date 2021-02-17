package mail

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"io/ioutil"
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
		fmt.Print(string(msg))
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
		mixedcontenttype, mixedparams, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
		assert.NoError(err)
		assert.Equal("multipart/mixed", mixedcontenttype)
		assert.Equal("utf-8", mixedparams["charset"])
		assert.NotEqual("", mixedparams["boundary"])
		r := multipart.NewReader(m.Body, mixedparams["boundary"])
		body, err := r.NextPart()
		assert.NoError(err)
		altcontenttype, altparams, err := mime.ParseMediaType(body.Header.Get("Content-Type"))
		assert.NoError(err)
		assert.Equal("multipart/alternative", altcontenttype)
		assert.NotEqual("", altparams["boundary"])
		b := multipart.NewReader(body, altparams["boundary"])
		plain, err := b.NextPart()
		assert.NoError(err)
		plaincontenttype, plainparams, err := mime.ParseMediaType(plain.Header.Get("Content-Type"))
		assert.NoError(err)
		assert.Equal("text/plain", plaincontenttype)
		assert.Equal("utf-8", plainparams["charset"])
		plainbytes, err := ioutil.ReadAll(plain)
		assert.NoError(err)
		assert.Equal(ti.body, string(plainbytes))
		htmlpart, err := b.NextPart()
		assert.NoError(err)
		htmlcontenttype, htmlparams, err := mime.ParseMediaType(htmlpart.Header.Get("Content-Type"))
		assert.NoError(err)
		assert.Equal("text/html", htmlcontenttype)
		assert.Equal("utf-8", htmlparams["charset"])
		htmlbytes, err := ioutil.ReadAll(htmlpart)
		assert.NoError(err)
		assert.Equal(ti.htmlbody, string(htmlbytes))
		_, err = b.NextPart()
		assert.Equal(io.EOF, err)
		_, err = r.NextPart()
		assert.Equal(io.EOF, err)
	}
}
