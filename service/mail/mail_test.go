package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMail(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test     string
		Subject  string
		From     string
		FromName string
		To       []string
		Body     string
		HtmlBody string
	}{
		{
			Test:     "text and html email with multiple recipients",
			Subject:  "Hello World",
			From:     "kevin@xorkevin.com",
			FromName: "Kevin Wang",
			To:       []string{"other@xorkevin.com", "other2@xorkevin.com"},
			Body:     "This is a test plain text alternate that goes over the line limit of 78 characters.",
			HtmlBody: "<html><body>This is some test html that goes over the line limit of 78 characters.</body></html>",
		},
	} {
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			msg, err := msgToBytes(tc.Subject, tc.From, tc.FromName, tc.To, []byte(tc.Body), []byte(tc.HtmlBody))
			assert.NoError(err)
			fmt.Print(string(msg))
			m, err := mail.ReadMessage(bytes.NewBuffer(msg))
			assert.NoError(err)
			assert.Equal(tc.Subject, m.Header.Get("Subject"))
			from, err := m.Header.AddressList("From")
			assert.NoError(err)
			assert.Len(from, 1)
			assert.Equal(tc.From, from[0].Address)
			assert.Equal(tc.FromName, from[0].Name)
			to, err := m.Header.AddressList("To")
			assert.NoError(err)
			assert.Len(to, len(tc.To))
			for n, i := range to {
				assert.Equal(tc.To[n], i.Address)
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
			plainbytes, err := io.ReadAll(plain)
			assert.NoError(err)
			assert.Equal(tc.Body, string(plainbytes))
			htmlpart, err := b.NextPart()
			assert.NoError(err)
			htmlcontenttype, htmlparams, err := mime.ParseMediaType(htmlpart.Header.Get("Content-Type"))
			assert.NoError(err)
			assert.Equal("text/html", htmlcontenttype)
			assert.Equal("utf-8", htmlparams["charset"])
			htmlbytes, err := io.ReadAll(htmlpart)
			assert.NoError(err)
			assert.Equal(tc.HtmlBody, string(htmlbytes))
			_, err = b.NextPart()
			assert.Equal(io.EOF, err)
			_, err = r.NextPart()
			assert.Equal(io.EOF, err)
		})
	}
}
