package mailinglist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	gomail "net/mail"
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
	id           string
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
	mailidRandSize = 16
)

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	u, err := uid.NewSnowflake(mailidRandSize)
	if err != nil {
		s.logger.Error("Failed to generate mail id", map[string]string{
			"cmd":      "mail",
			"mailfrom": from,
			"error":    err.Error(),
		})
		return errSMTPBase
	}
	id := u.Base32()
	l := s.logger.WithData(map[string]string{
		"cmd": "mail",
		"id":  id,
	})
	addr, err := gomail.ParseAddress(from)
	if err != nil {
		l.Warn("Failed to parse smtp from addr", map[string]string{
			"mailfrom": from,
			"error":    err.Error(),
		})
		return errSMTPFromAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		l.Warn("Failed to parse smtp from addr parts", map[string]string{
			"mailfrom": from,
		})
		return errSMTPFromAddr
	}
	if localPart == "" {
		l.Warn("Failed to parse smtp from addr parts", map[string]string{
			"mailfrom": from,
		})
		return errSMTPFromAddr
	}
	if domain == "" {
		l.Warn("Failed to parse smtp from addr parts", map[string]string{
			"mailfrom": from,
		})
		return errSMTPFromAddr
	}
	// DMARC requires checking RFC5321.MailFrom identity and not RFC5321.HELO
	result, spfErr, err := s.checkSPF(domain, from)
	if err != nil {
		l.Warn("Failed smtp from addr spf check", map[string]string{
			"from":       addr.Address,
			"spf_result": string(result),
			"error":      spfErr.Error(),
		})
		return err
	}
	s.id = id
	s.from = addr.Address
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
	l := s.logger.WithData(map[string]string{
		"cmd":  "to",
		"id":   s.id,
		"from": s.from,
		"to":   to,
	})
	if s.rcptTo != "" {
		l.Warn("Failed smtp rcpt count", map[string]string{
			"rcptTo": s.rcptTo,
		})
		return errSMTPRcptCount
	}
	addr, err := gomail.ParseAddress(to)
	if err != nil {
		l.Warn("Failed to parse smtp to addr", map[string]string{
			"error": err.Error(),
		})
		return errSMTPRcptAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		l.Warn("Failed to parse smtp to addr parts", nil)
		return errSMTPRcptAddr
	}
	listCreator, listname, ok := strings.Cut(localPart, mailboxKeySeparator)
	if !ok {
		l.Warn("Failed to parse smtp to addr parts", nil)
		return errSMTPMailbox
	}
	isOrg := domain == s.service.orgdomain
	if domain != s.service.usrdomain && !isOrg {
		l.Warn("Invalid smtp to domain", nil)
		return errSMTPSystem
	}

	var listCreatorID string
	if isOrg {
		creator, err := s.service.orgs.GetByName(context.Background(), listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				l.Warn("Owner org not found", map[string]string{
					"error": err.Error(),
				})
				return errSMTPMailbox
			}
			l.Error("Failed to get owner org by name", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBase
		}
		listCreatorID = rank.ToOrgName(creator.OrgID)
	} else {
		creator, err := s.service.users.GetByUsername(context.Background(), listCreator)
		if err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				l.Warn("Owner user not found", map[string]string{
					"error": err.Error(),
				})
				return errSMTPMailbox
			}
			l.Error("Failed to get owner user by name", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBase
		}
		listCreatorID = creator.Userid
	}

	l = l.WithData(map[string]string{
		"owner": listCreatorID,
	})

	list, err := s.service.lists.GetList(context.Background(), listCreatorID, listname)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			l.Warn("Mailbox not found", map[string]string{
				"error": err.Error(),
			})
			return errSMTPMailbox
		}
		l.Error("Failed to get mailbox", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBase
	}

	if list.Archive {
		l.Warn("Mailbox is archived", map[string]string{
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
	headerReceivedSPF           = "Received-SPF"
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
	l := s.logger.WithData(map[string]string{
		"cmd":    "data",
		"id":     s.id,
		"from":   s.from,
		"list":   s.rcptList,
		"msgid":  msgid,
		"sender": senderid,
		"policy": s.senderPolicy,
	})
	switch s.senderPolicy {
	case listSenderPolicyOwner:
		if s.isOrg {
			if ok, err := gate.AuthMember(context.Background(), s.service.gate, senderid, s.rcptOwner); err != nil {
				l.Error("Failed to auth org member", map[string]string{
					"error": err.Error(),
				})
				return errSMTPBase
			} else if !ok {
				l.Warn("Not allowed to send", nil)
				return errSMTPAuthSend
			}
		} else {
			if senderid != s.rcptOwner {
				l.Warn("Not allowed to send", nil)
				return errSMTPAuthSend
			}
			if ok, err := gate.AuthUser(context.Background(), s.service.gate, senderid); err != nil {
				l.Error("Failed to auth user", map[string]string{
					"error": err.Error(),
				})
				return errSMTPBase
			} else if !ok {
				l.Warn("Not allowed to send", nil)
				return errSMTPAuthSend
			}
		}
	case listSenderPolicyMember:
		if ok, err := gate.AuthUser(context.Background(), s.service.gate, senderid); err != nil {
			l.Error("Failed to auth user", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBase
		} else if !ok {
			l.Warn("Not allowed to send", nil)
			return errSMTPAuthSend
		}
		if _, err := s.service.lists.GetMember(context.Background(), s.rcptList, senderid); err != nil {
			if errors.Is(err, db.ErrNotFound{}) {
				l.Warn("List member not found", map[string]string{
					"error": err.Error(),
				})
				return errSMTPAuthSend
			}
			l.Error("Failed to get list member", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBase
		}
	case listSenderPolicyUser:
		if ok, err := gate.AuthUser(context.Background(), s.service.gate, senderid); err != nil {
			l.Error("Failed to auth user", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBase
		} else if !ok {
			l.Warn("Not allowed to send", nil)
			return errSMTPAuthSend
		}
	default:
		l.Warn("Invalid mailbox sender policy", nil)
		return errSMTPMailboxConfig
	}
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	l := s.logger.WithData(map[string]string{
		"cmd":  "data",
		"id":   s.id,
		"from": s.from,
		"list": s.rcptList,
	})
	if s.from == "" || s.rcptTo == "" {
		l.Warn("Failed smtp command sequence", nil)
		return errSMTPSeq
	}

	b := &bytes.Buffer{}
	if _, err := io.Copy(b, r); err != nil {
		l.Warn("Failed to read smtp data", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBaseExists
	}
	m, err := message.Read(bytes.NewReader(b.Bytes()))
	if err != nil {
		l.Warn("Failed to parse smtp data", map[string]string{
			"error": err.Error(),
		})
		return errMailBody
	}
	headers := emmail.Header{
		Header: m.Header,
	}

	msgid, err := headers.MessageID()
	if err != nil || msgid == "" {
		l.Warn("Failed to parse mail msgid", map[string]string{
			"error": err.Error(),
		})
		return errMailBody
	}

	l = l.WithData(map[string]string{
		"msgid": msgid,
	})

	contentType, _, err := headers.ContentType()
	if err != nil {
		l.Warn("Failed to parse mail content type", map[string]string{
			"error": err.Error(),
		})
		return errMailBody
	}

	fromAddrs, err := headers.AddressList(headerFrom)
	if err != nil || len(fromAddrs) == 0 {
		l.Warn("Failed to parse mail header from", map[string]string{
			"error": err.Error(),
		})
		return errMailBody
	}
	if len(fromAddrs) != 1 {
		l.Warn("Invalid mail header from", map[string]string{
			"error": err.Error(),
		})
		return errSPFAlignment
	}
	fromAddr := fromAddrs[0].Address
	localPart, fromAddrDomain, ok := strings.Cut(fromAddr, "@")
	if !ok {
		l.Warn("Failed to parse mail header from to parts", nil)
		return errMailBody
	}
	if localPart == "" {
		l.Warn("Failed to parse mail header from to parts", nil)
		return errMailBody
	}
	if fromAddrDomain == "" {
		l.Warn("Failed to parse mail header from to parts", nil)
		return errMailBody
	}

	if !s.isAligned(s.fromDomain, fromAddrDomain) {
		l.Warn("Failed spf alignment", map[string]string{
			"sender_addr": fromAddr,
		})
		return errSPFAlignment
	}

	sender, err := s.service.users.GetByEmail(context.Background(), fromAddr)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			l.Warn("Sender user not found", map[string]string{
				"error":       err.Error(),
				"sender_addr": fromAddr,
			})
			return errSMTPAuthSend
		}
		l.Error("Failed to get sender user by email", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBase
	}

	l = l.WithData(map[string]string{
		"sender": sender.Userid,
	})

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
		MaxVerifications: 0, // unlimited number of dkim signature verifications
	})
	if dkimErr != nil {
		dkimResults = nil
	}

	authResults := make([]authres.Result, 0, 3+len(dkimResults))
	var spfReason string
	var spfHeader string
	switch s.fromSPF {
	case authres.ResultPass:
		spfReason = fmt.Sprintf("%s: domain of %s designates %s as permitted sender", s.service.authdomain, s.from, s.srcip.String())
		spfHeader = fmt.Sprintf("pass (%s) client-ip=%s; envelope-from=%s; helo=%s; receiver=%s; identity=%s;", spfReason, s.srcip.String(), s.from, s.helo, s.service.authdomain, "mailfrom")
	case authres.ResultNeutral:
		spfReason = fmt.Sprintf("%s: %s is neither permitted nor denied by domain of %s", s.service.authdomain, s.srcip.String(), s.from)
		spfHeader = fmt.Sprintf("neutral (%s) client-ip=%s; envelope-from=%s; helo=%s; receiver=%s; identity=%s;", spfReason, s.srcip.String(), s.from, s.helo, s.service.authdomain, "mailfrom")
	default:
		spfReason = fmt.Sprintf("%s: domain of %s does not designate permitted sender hosts", s.service.authdomain, s.from)
		spfHeader = fmt.Sprintf("none (%s) client-ip=%s; envelope-from=%s; helo=%s; receiver=%s; identity=%s;", spfReason, s.srcip.String(), s.from, s.helo, s.service.authdomain, "mailfrom")
	}
	authResults = append(authResults, &authres.SPFResult{
		Value:  s.fromSPF,
		Reason: spfReason,
		From:   s.from,
	})
	if dkimErr != nil {
		authResults = append(authResults, &authres.DKIMResult{
			Value:  authres.ResultNeutral,
			Reason: "failed validating dkim signature",
		})
	} else if len(dkimResults) == 0 {
		authResults = append(authResults, &authres.DKIMResult{
			Value: authres.ResultNone,
		})
	}
	strictDKIMAlignment := dmarcErr == nil && dmarcRec.DKIMAlignment == dmarc.AlignmentStrict
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
				l.Warn("Failed spf alignment", map[string]string{
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
	m.Header.Add(headerReceivedSPF, spfHeader)
	m.Header.Add(headerReceived, fmt.Sprintf("from %s (%s [%s]) by %s with %s id %s for %s; %s", s.helo, s.helo, s.srcip.String(), s.service.authdomain, "ESMTPS", s.id, s.rcptTo, time.Now().Round(0).UTC().Format(headerReceivedTimeFormat)))

	mb := &bytes.Buffer{}
	if err := m.WriteTo(mb); err != nil {
		l.Error("Failed to write mail msg", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBaseExists
	}

	j, err := json.Marshal(mailmsg{
		ListID: s.rcptList,
		MsgID:  msgid,
	})
	if err != nil {
		l.Error("Failed to encode list event to json", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBaseExists
	}

	if msg, err := s.service.lists.GetMsg(context.Background(), s.rcptList, msgid); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			l.Error("Failed to get list msg", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBaseExists
		}
	} else {
		// mail already exists for this list
		if !msg.Processed {
			if err := s.service.events.StreamPublish(context.Background(), s.service.opts.MailChannel, j); err != nil {
				l.Error("Failed to publish list event", map[string]string{
					"error": err.Error(),
				})
				return errSMTPBaseExists
			}
			if err := s.service.lists.MarkMsgProcessed(context.Background(), s.rcptList, msgid); err != nil {
				l.Error("Failed to mark list message processed", map[string]string{
					"error": err.Error(),
				})
			}
		}
		return nil
	}

	if err := s.service.rcvMailDir.Subdir(s.rcptList).Put(context.Background(), s.service.encodeMsgid(msgid), contentType, int64(mb.Len()), nil, bytes.NewReader(mb.Bytes())); err != nil {
		l.Error("Failed to store mail msg", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBaseExists
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
	if err := s.service.lists.InsertMsg(context.Background(), msg); err != nil {
		if !errors.Is(err, db.ErrUnique{}) {
			l.Error("Failed to add list msg", map[string]string{
				"error": err.Error(),
			})
			return errSMTPBaseExists
		}
		// Message has already been added for this list, but not guaranteed to be
		// sent yet, hence must continue with publishing the event and marking
		// message as processed.
	}
	if err := s.service.events.StreamPublish(context.Background(), s.service.opts.MailChannel, j); err != nil {
		l.Error("Failed to publish list event", map[string]string{
			"error": err.Error(),
		})
		return errSMTPBaseExists
	}
	if err := s.service.lists.MarkMsgProcessed(context.Background(), s.rcptList, msgid); err != nil {
		l.Error("Failed to mark list message processed", map[string]string{
			"error": err.Error(),
		})
	}
	l.Info("Received mail", nil)
	return nil
}

func (s *smtpSession) Reset() {
	s.id = ""
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
