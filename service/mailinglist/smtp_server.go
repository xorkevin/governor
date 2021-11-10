package mailinglist

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-message"
	emmail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-msgauth/authres"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor"
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
	logger  governor.Logger
}

func (s *smtpBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

func (s *smtpBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	host, _, err := net.SplitHostPort(state.RemoteAddr.String())
	if err != nil {
		s.logger.Warn("Failed to parse smtp remote addr", map[string]string{
			"cmd":   "helo",
			"error": err.Error(),
		})
		return nil, errSMTPConn
	}
	hostip := net.ParseIP(host)
	if hostip == nil {
		s.logger.Warn("Failed to parse smtp remote addr ip", map[string]string{
			"cmd":   "helo",
			"error": err.Error(),
		})
		return nil, errSMTPConn
	}
	return &smtpSession{
		service: s.service,
		logger: s.logger.WithData(map[string]string{
			"session_ip":   hostip.String(),
			"session_helo": state.Hostname,
		}),
		srcip: hostip,
		helo:  state.Hostname,
	}, nil
}

type smtpSession struct {
	service      *service
	logger       governor.Logger
	srcip        net.IP
	helo         string
	from         string
	fromDomain   string
	fromSPF      authres.ResultValue
	rcptTo       string
	rcptList     string
	rcptOwner    string
	senderPolicy string
	isOrg        bool
}

func (s *smtpSession) checkSPF(domain, from string) (authres.ResultValue, error, error) {
	result, spfErr := spf.CheckHostWithSender(s.srcip, domain, from, spf.WithContext(context.Background()), spf.WithResolver(s.service.resolver))
	switch result {
	case spf.Pass:
		return authres.ResultPass, spfErr, nil
	case spf.Neutral:
		return authres.ResultNeutral, spfErr, nil
	case spf.None:
		return authres.ResultNone, spfErr, nil
	case spf.Fail:
		return authres.ResultFail, spfErr, errSPFFail
	case spf.SoftFail:
		return authres.ResultSoftFail, spfErr, errSPFFail
	case spf.TempError:
		return authres.ResultTempError, spfErr, errSPFTemp
	case spf.PermError:
		return authres.ResultPermError, spfErr, errSPFPerm
	default:
		return authres.ResultNone, spfErr, nil
	}
}

const (
	smtpIDRandSize = 16
)

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	addr, err := emmail.ParseAddress(from)
	if err != nil {
		s.logger.Warn("Failed to parse smtp from addr", map[string]string{
			"cmd":   "mail",
			"error": err.Error(),
			"from":  from,
		})
		return errSMTPFromAddr
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		s.logger.Warn("Failed to parse smtp from addr parts", map[string]string{
			"cmd":  "mail",
			"from": from,
		})
		return errSMTPFromAddr
	}
	if localPart := addrParts[0]; localPart == "" {
		s.logger.Warn("Failed to parse smtp from addr parts", map[string]string{
			"cmd":  "mail",
			"from": from,
		})
		return errSMTPFromAddr
	}
	domain := addrParts[1]
	if domain == "" {
		s.logger.Warn("Failed to parse smtp from addr parts", map[string]string{
			"cmd":  "mail",
			"from": from,
		})
		return errSMTPFromAddr
	}
	result, spfErr, err := s.checkSPF(domain, from)
	if err != nil {
		s.logger.Warn("Failed smtp from addr spf check", map[string]string{
			"cmd":        "mail",
			"from":       from,
			"error":      spfErr.Error(),
			"spf_result": string(result),
		})
		return err
	}
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
		s.logger.Warn("Failed smtp command sequence", map[string]string{
			"cmd": "to",
			"to":  to,
		})
		return errSMTPSeq
	}
	if s.rcptTo != "" {
		s.logger.Warn("Failed smtp rcpt count", map[string]string{
			"cmd":    "to",
			"from":   s.from,
			"to":     to,
			"rcptTo": s.rcptTo,
		})
		return errSMTPRcptCount
	}
	addr, err := emmail.ParseAddress(to)
	if err != nil {
		s.logger.Warn("Failed to parse smtp to addr", map[string]string{
			"cmd":   "to",
			"error": err.Error(),
			"from":  s.from,
			"to":    to,
		})
		return errSMTPRcptAddr
	}
	addrParts := strings.Split(addr.Address, "@")
	if len(addrParts) != 2 {
		s.logger.Warn("Failed to parse smtp to addr parts", map[string]string{
			"cmd":  "to",
			"from": s.from,
			"to":   to,
		})
		return errSMTPRcptAddr
	}
	mailboxParts := strings.Split(addrParts[0], mailboxKeySeparator)
	domain := addrParts[1]
	if len(mailboxParts) != 2 {
		s.logger.Warn("Failed to parse smtp to addr parts", map[string]string{
			"cmd":  "to",
			"from": s.from,
			"to":   to,
		})
		return errSMTPMailbox
	}
	listCreator := mailboxParts[0]
	listname := mailboxParts[1]
	if domain != s.service.usrdomain && domain != s.service.orgdomain {
		s.logger.Warn("Invalid smtp to domain", map[string]string{
			"cmd":  "to",
			"from": s.from,
			"to":   to,
		})
		return errSMTPSystem
	}
	isOrg := domain == s.service.orgdomain

	var listCreatorID string
	if isOrg {
		creator, err := s.service.orgs.GetByName(listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				s.logger.Warn("Owner org not found", map[string]string{
					"cmd":   "to",
					"error": err.Error(),
					"from":  s.from,
					"to":    to,
				})
				return errSMTPMailbox
			}
			s.logger.Error("Failed to get owner org by name", map[string]string{
				"cmd":   "to",
				"error": err.Error(),
				"from":  s.from,
				"to":    to,
			})
			return errSMTPBase
		}
		listCreatorID = rank.ToOrgName(creator.OrgID)
	} else {
		creator, err := s.service.users.GetByUsername(listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				s.logger.Warn("Owner user not found", map[string]string{
					"cmd":   "to",
					"error": err.Error(),
					"from":  s.from,
					"to":    to,
				})
				return errSMTPMailbox
			}
			s.logger.Error("Failed to get owner user by name", map[string]string{
				"cmd":   "to",
				"error": err.Error(),
				"from":  s.from,
				"to":    to,
			})
			return errSMTPBase
		}
		listCreatorID = creator.Userid
	}

	list, err := s.service.lists.GetList(listCreatorID, listname)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Warn("Mailbox not found", map[string]string{
				"cmd":   "to",
				"error": err.Error(),
				"from":  s.from,
				"to":    to,
				"owner": listCreatorID,
			})
			return errSMTPMailbox
		}
		s.logger.Error("Failed to get mailbox", map[string]string{
			"cmd":   "to",
			"error": err.Error(),
			"from":  s.from,
			"to":    to,
			"owner": listCreatorID,
		})
		return errSMTPBase
	}

	if list.Archive {
		s.logger.Warn("Mailbox is archived", map[string]string{
			"cmd":  "to",
			"from": s.from,
			"list": list.ListID,
		})
		return errSMTPMailboxDisabled
	}

	s.rcptTo = to
	s.rcptList = list.ListID
	s.rcptOwner = list.CreatorID
	s.senderPolicy = list.SenderPolicy
	s.isOrg = isOrg
	return nil
}

const (
	headerMessageID             = "Message-ID"
	headerFrom                  = "From"
	headerInReplyTo             = "In-Reply-To"
	headerAuthenticationResults = "Authentication-Results"
	headerReceived              = "Received"
	headerReceivedTimeFormat    = "Mon, 02 Jan 2006 15:04:05 -0700 (MST)"
)

const (
	maxSubjectLength = 127
)

func (s *smtpSession) isAligned(a, b string) bool {
	return strings.HasSuffix(a, b) || strings.HasSuffix(b, a)
}

func (s *smtpSession) checkListPolicy(senderid string, msgid string) error {
	switch s.senderPolicy {
	case listSenderPolicyOwner:
		if s.isOrg {
			if ok, err := gate.AuthMember(s.service.gate, senderid, s.rcptOwner); err != nil {
				s.logger.Error("Failed to auth org member", map[string]string{
					"cmd":    "to",
					"error":  err.Error(),
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPBase
			} else if !ok {
				s.logger.Warn("Not allowed to send", map[string]string{
					"cmd":    "to",
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPAuthSend
			}
		} else {
			if senderid != s.rcptOwner {
				s.logger.Warn("Not allowed to send", map[string]string{
					"cmd":    "to",
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPAuthSend
			}
			if ok, err := gate.AuthUser(s.service.gate, senderid); err != nil {
				s.logger.Error("Failed to auth user", map[string]string{
					"cmd":    "to",
					"error":  err.Error(),
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPBase
			} else if !ok {
				s.logger.Warn("Not allowed to send", map[string]string{
					"cmd":    "to",
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPAuthSend
			}
		}
	case listSenderPolicyMember:
		if ok, err := gate.AuthUser(s.service.gate, senderid); err != nil {
			s.logger.Error("Failed to auth user", map[string]string{
				"cmd":    "to",
				"error":  err.Error(),
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": senderid,
				"policy": s.senderPolicy,
			})
			return errSMTPBase
		} else if !ok {
			s.logger.Warn("Not allowed to send", map[string]string{
				"cmd":    "to",
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": senderid,
				"policy": s.senderPolicy,
			})
			return errSMTPAuthSend
		}
		if _, err := s.service.lists.GetMember(s.rcptList, senderid); err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				s.logger.Warn("List member not found", map[string]string{
					"cmd":    "to",
					"error":  err.Error(),
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": senderid,
					"policy": s.senderPolicy,
				})
				return errSMTPAuthSend
			}
			s.logger.Error("Failed to get list member", map[string]string{
				"cmd":    "to",
				"error":  err.Error(),
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": senderid,
				"policy": s.senderPolicy,
			})
			return errSMTPBase
		}
	case listSenderPolicyUser:
		if ok, err := gate.AuthUser(s.service.gate, senderid); err != nil {
			s.logger.Error("Failed to auth user", map[string]string{
				"cmd":    "to",
				"error":  err.Error(),
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": senderid,
				"policy": s.senderPolicy,
			})
			return errSMTPBase
		} else if !ok {
			s.logger.Warn("Not allowed to send", map[string]string{
				"cmd":    "to",
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": senderid,
				"policy": s.senderPolicy,
			})
			return errSMTPAuthSend
		}
	default:
		s.logger.Warn("Invalid mailbox sender policy", map[string]string{
			"cmd":    "to",
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": senderid,
			"policy": s.senderPolicy,
		})
		return errSMTPMailboxConfig
	}
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	if s.from == "" || s.rcptTo == "" {
		s.logger.Warn("Failed smtp command sequence", map[string]string{
			"cmd":  "data",
			"from": s.from,
			"list": s.rcptList,
		})
		return errSMTPSeq
	}

	b := bytes.Buffer{}
	if _, err := io.Copy(&b, r); err != nil {
		s.logger.Warn("Failed to read smtp data", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
		})
		return errSMTPBaseExists
	}
	m, err := message.Read(bytes.NewReader(b.Bytes()))
	if err != nil {
		s.logger.Warn("Failed to parse smtp data", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
		})
		return errMailBody
	}
	headers := emmail.Header{
		Header: m.Header,
	}

	msgid, err := headers.MessageID()
	if err != nil || msgid == "" {
		s.logger.Warn("Failed to parse mail msgid", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
		})
		return errMailBody
	}
	contentType, _, err := headers.ContentType()
	if err != nil {
		s.logger.Warn("Failed to parse mail content type", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errMailBody
	}

	fromAddrs, err := headers.AddressList(headerFrom)
	if err != nil || len(fromAddrs) == 0 {
		s.logger.Warn("Failed to parse mail header from", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errMailBody
	}
	if len(fromAddrs) != 1 {
		s.logger.Warn("Invalid mail header from", map[string]string{
			"cmd":   "data",
			"error": err.Error(),
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errSPFAlignment
	}
	fromAddr := fromAddrs[0].Address
	fromAddrParts := strings.Split(fromAddr, "@")
	if len(fromAddrParts) != 2 {
		s.logger.Warn("Failed to parse mail header from to parts", map[string]string{
			"cmd":   "data",
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errMailBody
	}
	if localPart := fromAddrParts[0]; localPart == "" {
		s.logger.Warn("Failed to parse mail header from to parts", map[string]string{
			"cmd":   "data",
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errMailBody
	}
	fromAddrDomain := fromAddrParts[1]
	if fromAddrDomain == "" {
		s.logger.Warn("Failed to parse mail header from to parts", map[string]string{
			"cmd":   "data",
			"from":  s.from,
			"list":  s.rcptList,
			"msgid": msgid,
		})
		return errMailBody
	}
	if !s.isAligned(s.fromDomain, fromAddrDomain) {
		s.logger.Warn("Failed spf alignment", map[string]string{
			"cmd":         "data",
			"from":        s.from,
			"list":        s.rcptList,
			"msgid":       msgid,
			"sender_addr": fromAddr,
		})
		return errSPFAlignment
	}

	sender, err := s.service.users.GetByEmail(fromAddr)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Warn("Sender user not found", map[string]string{
				"cmd":         "to",
				"error":       err.Error(),
				"from":        s.from,
				"list":        s.rcptList,
				"msgid":       msgid,
				"sender_addr": fromAddr,
			})
			return errSMTPAuthSend
		}
		s.logger.Error("Failed to get sender user by email", map[string]string{
			"cmd":         "to",
			"error":       err.Error(),
			"from":        s.from,
			"list":        s.rcptList,
			"msgid":       msgid,
			"sender_addr": fromAddr,
		})
		return errSMTPBase
	}

	if err := s.checkListPolicy(sender.Userid, msgid); err != nil {
		return err
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
	spfReason := ""
	if s.fromSPF != authres.ResultNone {
		spfReason = fmt.Sprintf("%s designates %s as a permitted sender", s.from, s.srcip.String())
	}
	authResults = append(authResults, &authres.SPFResult{
		Value:  s.fromSPF,
		Reason: spfReason,
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
				s.logger.Warn("Failed spf alignment", map[string]string{
					"cmd":    "data",
					"from":   s.from,
					"list":   s.rcptList,
					"msgid":  msgid,
					"sender": sender.Userid,
					"policy": string(dmarcRec.Policy),
				})
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
	m.Header.Add(headerReceived, fmt.Sprintf("from %s (%s [%s]) by %s with %s id %s for %s; %s", s.helo, s.helo, s.srcip.String(), s.service.authdomain, "ESMTP", msgid, s.rcptTo, time.Now().Round(0).UTC().Format(headerReceivedTimeFormat)))

	mb := bytes.Buffer{}
	if err := m.WriteTo(&mb); err != nil {
		s.logger.Error("Failed to write mail msg", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}

	members, err := s.service.lists.GetMembers(s.rcptList, mailingListMemberAmountCap, 0)
	if err != nil {
		s.logger.Error("Failed to get list members", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}
	memberUserids := make([]string, 0, len(members))
	for _, i := range members {
		memberUserids = append(memberUserids, i.Userid)
	}
	recipients, err := s.service.users.GetInfoBulk(memberUserids)
	if err != nil {
		s.logger.Error("Failed to get list member users", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}
	rcpts := make([]string, 0, len(recipients.Users))
	for _, i := range recipients.Users {
		rcpts = append(rcpts, i.Email)
	}

	if _, err := s.service.lists.GetMsg(s.rcptList, msgid); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("Failed to get list msg", map[string]string{
				"cmd":    "data",
				"error":  err.Error(),
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": sender.Userid,
			})
			return errSMTPBaseExists
		}
	} else {
		// mail already exists for this list
		return nil
	}

	if err := s.service.rcvMailDir.Subdir(s.rcptList).Put(base64.RawURLEncoding.EncodeToString([]byte(msgid)), contentType, int64(mb.Len()), nil, bytes.NewReader(mb.Bytes())); err != nil {
		s.logger.Error("Failed to store mail msg", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}
	if len(rcpts) > 0 {
		if err := s.service.mailer.FwdStream(s.from, rcpts, int64(mb.Len()), bytes.NewReader(mb.Bytes()), false); err != nil {
			s.logger.Error("Failed to send mail msg", map[string]string{
				"cmd":    "data",
				"error":  err.Error(),
				"from":   s.from,
				"list":   s.rcptList,
				"msgid":  msgid,
				"sender": sender.Userid,
			})
			return errSMTPBaseExists
		}
	}
	msg := s.service.lists.NewMsg(s.rcptList, msgid, sender.Userid)
	if subject, err := headers.Subject(); err == nil {
		if len(subject) > maxSubjectLength {
			subject = subject[:maxSubjectLength]
		}
		msg.Subject = subject
	}
	if dmarcPassSPF {
		msg.SPFPass = s.fromDomain
	}
	if alignedDKIM != nil {
		msg.DKIMPass = alignedDKIM.Domain
	}
	if inReplyTo, err := headers.MsgIDList(headerInReplyTo); err == nil && len(inReplyTo) == 1 {
		msg.InReplyTo = inReplyTo[0]
	}
	if err := s.service.lists.InsertMsg(msg); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			// message has already been sent for this list
			return nil
		}
		s.logger.Error("Failed to add list msg", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}
	if err := s.service.lists.UpdateListLastUpdated(msg.ListID, msg.CreationTime); err != nil {
		s.logger.Error("Failed to update list last updated", map[string]string{
			"cmd":    "data",
			"error":  err.Error(),
			"from":   s.from,
			"list":   s.rcptList,
			"msgid":  msgid,
			"sender": sender.Userid,
		})
		return errSMTPBaseExists
	}
	s.logger.Debug("Received mail", map[string]string{
		"cmd":    "data",
		"from":   s.from,
		"list":   s.rcptList,
		"msgid":  msgid,
		"sender": sender.Userid,
	})
	// TODO: track threads
	return nil
}

func (s *smtpSession) Reset() {
	s.from = ""
	s.fromDomain = ""
	s.fromSPF = ""
	s.rcptTo = ""
	s.rcptList = ""
	s.rcptOwner = ""
	s.senderPolicy = ""
	s.isOrg = false
}

func (s *smtpSession) Logout() error {
	return nil
}
