package mailinglist

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/mail"
	"strings"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

var (
	errSMTPBase = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 0, 0},
		Message:      "Temporary error",
	}
	errSMTPBaseExists = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 2, 4},
		Message:      "Temporary error",
	}
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
	errSMTPMailboxConfig = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 3, 0},
		Message:      "Invalid recipient mailbox config",
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
	service *service
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
		service: s.service,
		srcip:   hostip,
	}, nil
}

type smtpSession struct {
	service  *service
	srcip    net.IP
	from     string
	rcptList string
	org      bool
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
	result, _ := spf.CheckHostWithSender(s.srcip, domain, from, spf.WithContext(context.Background()), spf.WithResolver(s.service.resolver))
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

const (
	mailingListMemberAmountCap = 255
	mailboxKeySeparator        = "."
	listSenderPolicyOwner      = "owner"
	listSenderPolicyMember     = "member"
	listSenderPolicyUser       = "user"
	listMemberPolicyOwner      = "owner"
	listMemberPolicyUser       = "user"
)

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
	mailboxParts := strings.Split(addrParts[0], mailboxKeySeparator)
	domain := addrParts[1]
	if len(mailboxParts) != 2 {
		return errSMTPMailbox
	}
	listCreator := mailboxParts[0]
	listname := mailboxParts[1]
	if domain != s.service.usrdomain && domain != s.service.orgdomain {
		return errSMTPSystem
	}
	isOrg := domain == s.service.orgdomain

	var listCreatorID string
	if isOrg {
		creator, err := s.service.orgs.GetByName(listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return errSMTPMailbox
			}
			return errSMTPBase
		}
		listCreatorID = rank.ToOrgName(creator.OrgID)
	} else {
		creator, err := s.service.users.GetByUsername(listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return errSMTPMailbox
			}
			return errSMTPBase
		}
		listCreatorID = creator.Userid
	}
	sender, err := s.service.users.GetByEmail(s.from)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return errSMTPAuthSend
		}
		return errSMTPBase
	}

	list, err := s.service.lists.GetList(listCreatorID, listname)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return errSMTPMailbox
		}
		return errSMTPBase
	}

	switch list.SenderPolicy {
	case listSenderPolicyOwner:
		if isOrg {
			if ok, err := gate.AuthMember(s.service.gate, sender.Userid, list.CreatorID); err != nil {
				return errSMTPBase
			} else if !ok {
				return errSMTPAuthSend
			}
		} else {
			if sender.Userid != list.CreatorID {
				return errSMTPAuthSend
			}
			if ok, err := gate.AuthUser(s.service.gate, sender.Userid); err != nil {
				return errSMTPBase
			} else if !ok {
				return errSMTPAuthSend
			}
		}
	case listSenderPolicyMember:
		if ok, err := gate.AuthUser(s.service.gate, sender.Userid); err != nil {
			return errSMTPBase
		} else if !ok {
			return errSMTPAuthSend
		}
		if _, err := s.service.lists.GetMember(listCreatorID, listname, sender.Userid); err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				return errSMTPAuthSend
			}
			return errSMTPBase
		}
	case listSenderPolicyUser:
		if ok, err := gate.AuthUser(s.service.gate, sender.Userid); err != nil {
			return errSMTPBase
		} else if !ok {
			return errSMTPAuthSend
		}
	default:
		return errSMTPMailboxConfig
	}

	s.rcptList = list.ListID
	s.org = isOrg

	members, err := s.service.lists.GetListMembers(listCreatorID, listname, mailingListMemberAmountCap, 0)
	if err != nil {
		return errSMTPBaseExists
	}
	userids := make([]string, 0, len(members))
	for _, i := range members {
		userids = append(userids, i.Userid)
	}
	rcpts, err := s.service.users.GetInfoBulk(userids)
	if err != nil {
		return errSMTPBaseExists
	}
	s.rcpts = make([]string, 0, len(rcpts.Users))
	for _, i := range rcpts.Users {
		s.rcpts = append(s.rcpts, i.Email)
	}
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
