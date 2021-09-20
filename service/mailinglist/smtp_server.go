package mailinglist

import (
	"io"
	"log"

	"github.com/emersion/go-smtp"
)

type smtpBackend struct{}

func (s *smtpBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

func (s *smtpBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &smtpSession{}, nil
}

type smtpSession struct {
	from  string
	rcpts []string
}

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	// TODO: check spf of from
	log.Println("Mail from:", from)
	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string) error {
	// TODO: verify recipient mailing address as target of from
	log.Println("Rcpt to:", to)
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	// TODO: check that recipients present and unique message id per recipient
	if b, err := io.ReadAll(r); err != nil {
		return err
	} else {
		log.Println("Data:", string(b))
	}
	return nil
}

func (s *smtpSession) Reset() {
	*s = smtpSession{}
}

func (s *smtpSession) Logout() error {
	return nil
}
