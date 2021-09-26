package mailinglist

import (
	"context"
	"io"
	"log"
	"net"
	"net/mail"
	"strings"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor/util/dns"
)

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
	errSMTPRcptAddr = &smtp.SMTPError{
		Code:         501,
		EnhancedCode: smtp.EnhancedCode{5, 1, 3},
		Message:      "Invalid recipient address",
	}
	errSMTPMailbox = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 1, 1},
		Message:      "Invalid recipient mailbox",
	}
	errSMTPSystem = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 1, 2},
		Message:      "Invalid recipient system",
	}
	errSMTPRcptCount = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 5, 3},
		Message:      "Too many recipients",
	}
	errSMTPAuthSend = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 2},
		Message:      "Unauthorized to send to this mailing list",
	}
	errSMTPSeq = &smtp.SMTPError{
		Code:         503,
		EnhancedCode: smtp.EnhancedCode{5, 5, 1},
		Message:      "Invalid command sequence",
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
	usrdomain string
	orgdomain string
	resolver  dns.Resolver
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
		usrdomain: s.usrdomain,
		orgdomain: s.orgdomain,
		resolver:  s.resolver,
		srcip:     hostip,
	}, nil
}

type smtpSession struct {
	usrdomain string
	orgdomain string
	resolver  dns.Resolver
	srcip     net.IP
	from      string
	rcptList  string
	org       bool
	rcpts     []string
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
	result, _ := spf.CheckHostWithSender(s.srcip, domain, from, spf.WithContext(context.Background()), spf.WithResolver(s.resolver))
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
	if s.from == "" {
		return errSMTPSeq
	}
	if s.rcptList != "" {
		return errSMTPRcptCount
	}
	addr, err := mail.ParseAddress(to)
	if err != nil {
		return errSMTPRcptAddr
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		return errSMTPRcptAddr
	}
	mailbox := addrParts[0]
	domain := addrParts[1]
	if domain != s.usrdomain && domain != s.orgdomain {
		return errSMTPSystem
	}
	// TODO: verify recipient mailing address as target of from, and set rcpts
	log.Println("Rcpt to:", to)
	s.rcptList = mailbox
	s.org = domain == s.orgdomain
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	if s.from == "" || s.rcptList == "" {
		return errSMTPSeq
	}
	// TODO: check that recipients present, message id not sent yet for list, and from alignment
	if b, err := io.ReadAll(r); err != nil {
		return err
	} else {
		log.Println("Data:", string(b))
	}
	return nil
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.rcptList = ""
	s.org = false
	s.rcpts = nil
}

func (s *smtpSession) Logout() error {
	return nil
}
