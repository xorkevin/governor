package mail

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	gomail "net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMail(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test    string
		MsgID   string
		Subject string
		From    Addr
		To      []Addr
		Body    string
	}{
		{
			Test:    "text email with multiple recipients",
			MsgID:   "msgid@mail.example.com",
			Subject: "Hello World",
			From: Addr{
				Address: "kevin@xorkevin.com",
				Name:    "Kevin Wang",
			},
			To: []Addr{
				{Address: "other@xorkevin.com"},
				{Address: "other2@xorkevin.com"},
			},
			Body: "This is a test plain text alternate that goes over the line limit of 78 characters.",
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()
			assert := require.New(t)

			buf := bytes.Buffer{}
			assert.NoError(msgToBytes(nil, context.Background(), tc.MsgID, tc.From, tc.To, strings.NewReader(tc.Subject), strings.NewReader(tc.Body), nil, &buf))
			t.Log(buf.String())
			m, err := gomail.ReadMessage(bytes.NewBuffer(buf.Bytes()))
			assert.NoError(err)
			assert.Equal("<"+tc.MsgID+">", m.Header.Get("Message-Id"))
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
			assert.Equal("quoted-printable", m.Header.Get("Content-Transfer-Encoding"))
			plaincontenttype, plainparams, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
			assert.NoError(err)
			assert.Equal("text/plain", plaincontenttype)
			assert.Equal("utf-8", plainparams["charset"])
			plainbytes, err := io.ReadAll(quotedprintable.NewReader(m.Body))
			assert.NoError(err)
			assert.Equal(tc.Body, string(plainbytes))
		})
	}

	for _, tc := range []struct {
		Test     string
		MsgID    string
		Subject  string
		From     Addr
		To       []Addr
		Body     string
		HtmlBody string
	}{
		{
			Test:    "text and html email with multiple recipients",
			Subject: "Hello World",
			MsgID:   "msgid@mail.example.com",
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
			assert.NoError(msgToBytes(nil, context.Background(), tc.MsgID, tc.From, tc.To, strings.NewReader(tc.Subject), strings.NewReader(tc.Body), strings.NewReader(tc.HtmlBody), &buf))
			t.Log(buf.String())
			m, err := gomail.ReadMessage(bytes.NewBuffer(buf.Bytes()))
			assert.NoError(err)
			assert.Equal("<"+tc.MsgID+">", m.Header.Get("Message-Id"))
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
