package mailinglist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-message"
	emmail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-msgauth/authres"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/governor/util/uid"
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
	errSMTPMailboxDisabled = &smtp.SMTPError{
		Code:         450,
		EnhancedCode: smtp.EnhancedCode{4, 2, 1},
		Message:      "Mailbox is archived",
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
	errDMARCPolicy = &smtp.SMTPError{
		Code:         550,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "Rejecting based on DMARC policy",
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
	service    *service
	srcip      net.IP
	helo       string
	id         string
	from       string
	fromDomain string
	fromSPF    authres.ResultValue
	fromUserid string
	rcptTo     string
	rcptList   string
	org        bool
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

const (
	smtpIDRandSize = 16
)

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	addr, err := emmail.ParseAddress(from)
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
	u, err := uid.NewSnowflake(smtpIDRandSize)
	if err != nil {
		return errSMTPBase
	}
	s.id = u.Base32()
	s.from = from
	s.fromDomain = domain
	s.fromSPF = result
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
	if s.rcptTo != "" {
		return errSMTPRcptCount
	}
	addr, err := emmail.ParseAddress(to)
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

	if list.Archive {
		return errSMTPMailboxDisabled
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
		if _, err := s.service.lists.GetMember(list.ListID, sender.Userid); err != nil {
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

	s.fromUserid = sender.Userid
	s.rcptTo = to
	s.rcptList = list.ListID
	s.org = isOrg
	return nil
}

const (
	headerMessageID             = "Message-ID"
	headerFrom                  = "From"
	headerAuthenticationResults = "Authentication-Results"
	headerReceived              = "Received"
	headerReceivedTimeFormat    = "Mon, 02 Jan 2006 15:04:05 -0700 (MST)"
)

func (s *smtpSession) isAligned(a, b string) bool {
	return strings.HasSuffix(a, b) || strings.HasSuffix(b, a)
}

func (s *smtpSession) Data(r io.Reader) error {
	if s.from == "" || s.rcptTo == "" {
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
	headers := emmail.Header{
		Header: m.Header,
	}

	msgid, err := headers.MessageID()
	if err != nil || msgid == "" {
		return errMailBody
	}
	contentType, _, err := headers.ContentType()
	if err != nil {
		return errMailBody
	}

	fromAddrs, err := headers.AddressList(headerFrom)
	if err != nil || len(fromAddrs) == 0 {
		return errMailBody
	}
	if len(fromAddrs) != 1 {
		return errSPFAlignment
	}
	fromAddrParts := strings.Split(fromAddrs[0].Address, "@")
	if len(fromAddrParts) != 2 {
		return errMailBody
	}
	if localPart := fromAddrParts[0]; localPart == "" {
		return errMailBody
	}
	fromAddrDomain := fromAddrParts[1]
	if fromAddrDomain == "" {
		return errMailBody
	}
	if !s.isAligned(s.fromDomain, fromAddrDomain) {
		return errSPFAlignment
	}

	dmarcRec, dmarcErr := dmarc.LookupWithOptions(fromAddrDomain, &dmarc.LookupOptions{
		LookupTXT: func(domain string) ([]string, error) {
			return s.service.resolver.LookupTXT(context.Background(), domain)
		},
	})

	dmarcPassSPF := s.fromSPF == authres.ResultPass
	if dmarcErr == nil && dmarcRec.SPFAlignment == dmarc.AlignmentStrict {
		if s.fromDomain != fromAddrDomain {
			dmarcPassSPF = false
		}
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

	authResults := make([]authres.Result, 0, 3+len(dkimResults))
	authResults = append(authResults, &authres.SPFResult{
		Value:  s.fromSPF,
		Reason: fmt.Sprintf("%s designates %s as a permitted sender", s.from, s.srcip.String()),
		From:   s.from,
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
	strictDKIMAlignment := false
	if dmarcErr == nil {
		strictDKIMAlignment = dmarcRec.DKIMAlignment == dmarc.AlignmentStrict
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
		} else {
			if strictDKIMAlignment {
				if i.Domain == fromAddrDomain {
					alignedDKIM = i
				}
			} else {
				if s.isAligned(i.Domain, fromAddrDomain) {
					alignedDKIM = i
				}
			}
		}
		authResults = append(authResults, &authres.DKIMResult{
			Value:      res,
			Domain:     i.Domain,
			Identifier: i.Identifier,
		})
	}
	if dmarcErr != nil {
		var res authres.ResultValue = authres.ResultPermError
		if errors.Is(dmarcErr, dmarc.ErrNoPolicy) {
			res = authres.ResultNone
		} else if dmarc.IsTempFail(dmarcErr) {
			res = authres.ResultTempError
		}
		authResults = append(authResults, &authres.DMARCResult{
			Value: res,
			From:  fromAddrDomain,
		})
	} else {
		var res authres.ResultValue = authres.ResultPass
		if !dmarcPassSPF && alignedDKIM == nil {
			if dmarcRec.Policy != dmarc.PolicyNone {
				return errDMARCPolicy
			}
			res = authres.ResultFail
		}
		authResults = append(authResults, &authres.DMARCResult{
			Value:  res,
			Reason: fmt.Sprintf("p=%s", dmarcRec.Policy),
			From:   fromAddrDomain,
		})
	}
	m.Header.Add(headerAuthenticationResults, authres.Format(s.service.authdomain, authResults))
	m.Header.Add(headerReceived, fmt.Sprintf("from %s (%s [%s]) by %s with %s id %s for %s; %s", s.helo, s.helo, s.srcip.String(), s.service.authdomain, "ESMTP", s.id, s.rcptTo, time.Now().Round(0).UTC().Format(headerReceivedTimeFormat)))

	mb := bytes.Buffer{}
	if err := m.WriteTo(&mb); err != nil {
		return errSMTPBaseExists
	}

	members, err := s.service.lists.GetMembers(s.rcptList, mailingListMemberAmountCap, 0)
	if err != nil {
		return errSMTPBaseExists
	}
	memberUserids := make([]string, 0, len(members))
	for _, i := range members {
		memberUserids = append(memberUserids, i.Userid)
	}
	recipients, err := s.service.users.GetInfoBulk(memberUserids)
	if err != nil {
		return errSMTPBaseExists
	}
	rcpts := make([]string, 0, len(recipients.Users))
	for _, i := range recipients.Users {
		rcpts = append(rcpts, i.Email)
	}

	if _, err := s.service.lists.GetMsg(s.rcptList, msgid); err != nil {
		if !errors.Is(err, &db.ErrNotFound{}) {
			return errSMTPBaseExists
		}
	} else {
		// mail already exists for this list
		return nil
	}

	if err := s.service.rcvMailDir.Subdir(s.rcptList).Put(url.QueryEscape(msgid), contentType, int64(mb.Len()), nil, bytes.NewReader(mb.Bytes())); err != nil {
		return errSMTPBaseExists
	}
	if len(rcpts) > 0 {
		if err := s.service.mailer.FwdStream("", rcpts, int64(mb.Len()), bytes.NewReader(mb.Bytes()), false); err != nil {
			return errSMTPBaseExists
		}
	}
	msg := s.service.lists.NewMsg(s.rcptList, msgid, s.fromUserid)
	if err := s.service.lists.InsertMsg(msg); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			// message has already been sent for this list
			return nil
		}
		return errSMTPBaseExists
	}
	if err := s.service.lists.UpdateListLastUpdated(msg.ListID, msg.CreationTime); err != nil {
		return errSMTPBaseExists
	}
	// TODO: track threads
	return nil
}

func (s *smtpSession) Reset() {
	s.id = ""
	s.from = ""
	s.fromDomain = ""
	s.fromSPF = ""
	s.fromUserid = ""
	s.rcptTo = ""
	s.rcptList = ""
	s.org = false
}

func (s *smtpSession) Logout() error {
	return nil
}
