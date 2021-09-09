package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	gomail "net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMail(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test     string
		Subject  string
		Sender   string
		From     Addr
		To       []Addr
		Body     string
		HtmlBody string
	}{
		{
			Test:    "text and html email with multiple recipients",
			Subject: "Hello World",
			Sender:  "example.com",
			From: Addr{
				Address: "kevin@xorkevin.com",
				Name:    "Kevin Wang",
			},
			To: []Addr{
				{Address: "other@xorkevin.com"},
				{Address: "other2@xorkevin.com"},
			},
			Body:     "This is a test plain text alternate that goes over the line limit of 78 characters.",
			HtmlBody: "<html><body>This is some test html that goes over the line limit of 78 characters.</body></html>",
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			buf := bytes.Buffer{}
			assert.NoError(msgToBytes(nil, tc.Sender, tc.Subject, tc.From, tc.To, []byte(tc.Body), []byte(tc.HtmlBody), &buf))
			t.Log(buf.String())
			m, err := gomail.ReadMessage(bytes.NewBuffer(buf.Bytes()))
			assert.NoError(err)
			assert.Equal(tc.Subject, m.Header.Get("Subject"))
			from, err := m.Header.AddressList("From")
			assert.NoError(err)
			assert.Len(from, 1)
			assert.Equal(tc.From.Address, from[0].Address)
			assert.Equal(tc.From.Name, from[0].Name)
			to, err := m.Header.AddressList("To")
			assert.NoError(err)
			assert.Len(to, len(tc.To))
			for n, i := range to {
				assert.Equal(tc.To[n].Address, i.Address)
			}
			assert.Equal("1.0", m.Header.Get("Mime-Version"))
			date, err := m.Header.Date()
			assert.NoError(err)
			assert.False(date.IsZero())
			{
				id := m.Header.Get("Message-Id")
				assert.True(strings.HasPrefix(id, "<"))
				assert.True(strings.HasSuffix(id, ">"))
				id = strings.TrimSuffix(strings.TrimPrefix(id, "<"), ">")
				parts := strings.Split(id, "@")
				assert.Len(parts, 2)
				assert.Equal(tc.Sender, parts[1])
			}
			mixedcontenttype, mixedparams, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
			assert.NoError(err)
			assert.Equal("multipart/mixed", mixedcontenttype)
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
