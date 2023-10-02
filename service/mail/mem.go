package mail

import (
	"bytes"
	"context"
	"io"
	"slices"

	"xorkevin.dev/governor/util/kjson"
)

var _ Mailer = (*MemLog)(nil)

type (
	MemLog struct {
		Records []LogRecord
	}

	LogRecord struct {
		RetPath string
		To      []Addr
		BodyRaw *bytes.Buffer
		From    Addr
		Subject string
		BodyTxt *bytes.Buffer
		Tpl     Tpl
		TplData map[string]string
		Encrypt bool
	}
)

func (s *MemLog) FwdStream(ctx context.Context, retpath string, to []Addr, size int64, body io.Reader, encrypt bool) error {
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, body); err != nil {
		return err
	}
	s.Records = append(s.Records, LogRecord{
		RetPath: retpath,
		To:      slices.Clone(to),
		BodyRaw: buf,
		Encrypt: encrypt,
	})
	return nil
}

func (s *MemLog) SendStream(ctx context.Context, retpath string, from Addr, to []Addr, subject string, size int64, body io.Reader, encrypt bool) error {
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, body); err != nil {
		return err
	}
	s.Records = append(s.Records, LogRecord{
		RetPath: retpath,
		To:      slices.Clone(to),
		From:    from,
		Subject: subject,
		BodyTxt: buf,
		Encrypt: encrypt,
	})
	return nil
}

func (s *MemLog) SendTpl(ctx context.Context, retpath string, from Addr, to []Addr, tpl Tpl, emdata interface{}, encrypt bool) error {
	b, err := kjson.Marshal(emdata)
	if err != nil {
		return err
	}
	var data map[string]string
	if err := kjson.Unmarshal(b, &data); err != nil {
		return err
	}
	s.Records = append(s.Records, LogRecord{
		RetPath: retpath,
		To:      slices.Clone(to),
		From:    from,
		Tpl:     tpl,
		TplData: data,
		Encrypt: encrypt,
	})
	return nil
}
