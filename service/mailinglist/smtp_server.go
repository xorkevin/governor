package mailinglist

import (
	"io"
	"log"
	"net"
	"net/mail"
	"strings"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor/util/dns"
)

type (
	// ErrSMTPNetwork is returned when receiving an unexpected smtp network conn
	ErrSMTPNetwork struct{}
)

func (e ErrSMTPNetwork) Error() string {
	return "Error SMTP network"
}

var (
	errSMTPConn = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 0, 0},
		Message:      "Invalid client ip address",
	}
	errSMTPFromAddr = &smtp.SMTPError{
		Code:         501,
		EnhancedCode: smtp.EnhancedCode{5, 1, 7},
		Message:      "Invalid mail from address",
	}
	errSPFFail = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "Failed spf",
	}
	errSPFTemp = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 4, 3},
		Message:      "Temporary spf error",
	}
	errSPFPerm = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 5, 2},
		Message:      "Invalid spf dns record",
	}
)

type smtpBackend struct {
	resolver dns.Resolver
}

func (s *smtpBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

func (s *smtpBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	host, _, err := net.SplitHostPort(state.RemoteAddr.String())
	if err != nil {
		return nil, errSMTPConn
	}
	hostip := net.ParseIP(host)
	if hostip == nil {
		return nil, errSMTPConn
	}
	return &smtpSession{
		resolver: s.resolver,
		srcip:    hostip,
	}, nil
}

type smtpSession struct {
	resolver dns.Resolver
	srcip    net.IP
	from     string
	rcpts    []string
}

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return errSMTPFromAddr
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		return errSMTPFromAddr
	}
	domain := addrParts[1]
	result, _ := spf.CheckHostWithSender(s.srcip, domain, from)
	switch result {
	case spf.Pass, spf.Neutral, spf.None:
	case spf.Fail, spf.SoftFail:
		return errSPFFail
	case spf.TempError:
		return errSPFTemp
	case spf.PermError:
		return errSPFPerm
	}
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
	s.from = ""
	s.rcpts = nil
}

func (s *smtpSession) Logout() error {
	return nil
}
