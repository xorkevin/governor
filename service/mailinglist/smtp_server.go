package mailinglist

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	gomail "net/mail"
	"strings"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-message"
	"github.com/emersion/go-msgauth/authres"
	"github.com/emersion/go-msgauth/dkim"
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
	errDKIMFail = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 7},
		Message:      "Failed DKIM verification",
	}
	errMailBody = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 7},
		Message:      "Malformed mail body",
	}
	errSPFAlignment = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "Failed SPF from header alignment",
	}
	errDKIMAlignment = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "Failed DKIM from header alignment",
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
		helo:    state.Hostname,
	}, nil
}

type smtpSession struct {
	service          *service
	srcip            net.IP
	helo             string
	from             string
	fromDomain       string
	fromSPF          authres.ResultValue
	rcptList         string
	org              bool
	headerFrom       string
	headerFromDomain string
	alignedDKIM      *dkim.Verification
}

func (s *smtpSession) checkSPF(domain, from string) (authres.ResultValue, error) {
	result, _ := spf.CheckHostWithSender(s.srcip, domain, from, spf.WithContext(context.Background()), spf.WithResolver(s.service.resolver))
	switch result {
	case spf.Pass:
		return authres.ResultPass, nil
	case spf.Neutral:
		return authres.ResultNeutral, nil
	case spf.None:
		return authres.ResultNone, nil
	case spf.Fail:
		return authres.ResultFail, errSPFFail
	case spf.SoftFail:
		return authres.ResultSoftFail, errSPFFail
	case spf.TempError:
		return authres.ResultTempError, errSPFTemp
	case spf.PermError:
		return authres.ResultPermError, errSPFPerm
	default:
		return authres.ResultNone, nil
	}
}

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	addr, err := gomail.ParseAddress(from)
	if err != nil {
		return errSMTPFromAddr
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		return errSMTPFromAddr
	}
	if localPart := addrParts[0]; localPart == "" {
		return errSMTPFromAddr
	}
	domain := addrParts[1]
	if domain == "" {
		return errSMTPFromAddr
	}
	result, err := s.checkSPF(domain, from)
	if err != nil {
		return err
	}
	s.fromSPF = result
	s.from = from
	s.fromDomain = domain
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
	addr, err := gomail.ParseAddress(to)
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
	recipients, err := s.service.users.GetInfoBulk(userids)
	if err != nil {
		return errSMTPBaseExists
	}
	rcpts := make([]string, 0, len(recipients.Users))
	for _, i := range recipients.Users {
		rcpts = append(rcpts, i.Email)
	}
	return nil
}

const (
	headerAuthenticationResults = "Authentication-Results"
)

func (s *smtpSession) isAligned(a, b string) bool {
	return strings.HasSuffix(a, b) || strings.HasSuffix(b, a)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *smtpSession) Data(r io.Reader) error {
	if s.from == "" || s.rcptList == "" {
		return errSMTPSeq
	}

	b := bytes.Buffer{}
	if _, err := io.Copy(&b, r); err != nil {
		return errSMTPBaseExists
	}
	m, err := message.Read(bytes.NewReader(b.Bytes()))
	if err != nil {
		return errMailBody
	}
	if m.Header.FieldsByKey("From").Len() != 1 {
		return errSPFAlignment
	}
	headerFrom := m.Header.Get("From")
	if headerFrom == "" {
		return errSPFAlignment
	}
	addr, err := gomail.ParseAddress(headerFrom)
	if err != nil {
		return errMailBody
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		return errMailBody
	}
	if localPart := addrParts[0]; localPart == "" {
		return errMailBody
	}
	headerFromDomain := addrParts[1]
	if headerFromDomain == "" {
		return errMailBody
	}
	if !s.isAligned(s.fromDomain, headerFromDomain) {
		return errSPFAlignment
	}
	dkimResults, dkimErr := dkim.VerifyWithOptions(bytes.NewReader(b.Bytes()), &dkim.VerifyOptions{
		LookupTXT: func(domain string) ([]string, error) {
			return s.service.resolver.LookupTXT(context.Background(), domain)
		},
		MaxVerifications: 0, // unlimited
	})
	if dkimErr != nil {
		dkimResults = nil
	}

	authResults := make([]authres.Result, 0, max(1+len(dkimResults), 2))
	authResults = append(authResults, &authres.SPFResult{
		Value: s.fromSPF,
		From:  s.from,
	})
	if dkimErr != nil {
		authResults = append(authResults, &authres.DKIMResult{
			Value:  authres.ResultNeutral,
			Reason: "failed processing dkim signature",
		})
	} else if len(dkimResults) == 0 {
		authResults = append(authResults, &authres.DKIMResult{
			Value: authres.ResultNone,
		})
	}
	var alignedDKIM *dkim.Verification
	for _, i := range dkimResults {
		var res authres.ResultValue = authres.ResultPass
		if i.Err != nil {
			if dkim.IsTempFail(i.Err) {
				res = authres.ResultTempError
			} else if dkim.IsPermFail(i.Err) {
				res = authres.ResultPermError
			} else {
				res = authres.ResultFail
			}
		} else if s.isAligned(i.Domain, headerFromDomain) {
			alignedDKIM = i
		}
		authResults = append(authResults, &authres.DKIMResult{
			Value:      res,
			Domain:     i.Domain,
			Identifier: i.Identifier,
		})
	}
	m.Header.Add(headerAuthenticationResults, authres.Format(s.service.authdomain, authResults))

	s.headerFrom = headerFrom
	s.headerFromDomain = headerFromDomain
	s.alignedDKIM = alignedDKIM
	// TODO: check message id not sent yet for list and dmarc alignment policy
	return nil
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.fromDomain = ""
	s.fromSPF = ""
	s.rcptList = ""
	s.org = false
	s.headerFrom = ""
	s.headerFromDomain = ""
	s.alignedDKIM = nil
}

func (s *smtpSession) Logout() error {
	return nil
}
