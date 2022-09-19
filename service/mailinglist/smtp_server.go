package mailinglist

import (
	"bytes"
	"context"
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
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
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
	log     *klog.LevelLogger
}

func (s *smtpBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return nil, smtp.ErrAuthUnsupported
}

func (s *smtpBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	ctx := klog.WithFields(context.Background(), klog.Fields{
		"smtp.cmd": "helo",
	})
	host, _, err := net.SplitHostPort(state.RemoteAddr.String())
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp remote addr"), nil)
		return nil, errSMTPConn
	}
	hostip := net.ParseIP(host)
	if hostip == nil {
		s.log.Warn(ctx, "Failed to parse smtp remote addr ip", klog.Fields{
			"smtp.host": host,
		})
		return nil, errSMTPConn
	}
	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.session.ip":   hostip.String(),
		"smtp.session.helo": state.Hostname,
	})
	return &smtpSession{
		service: s.service,
		log:     klog.NewLevelLogger(s.log.Logger.Sublogger("session", nil)),
		ctx:     ctx,
		srcip:   hostip,
		helo:    state.Hostname,
	}, nil
}

type smtpSession struct {
	service      *service
	log          *klog.LevelLogger
	ctx          context.Context
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

func (s *smtpSession) checkSPF(ctx context.Context, domain, from string) (authres.ResultValue, error, error) {
	result, spfErr := spf.CheckHostWithSender(s.srcip, domain, from, spf.WithContext(ctx), spf.WithResolver(s.service.resolver))
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
	mailidRandSize = 8
)

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	ctx := klog.WithFields(s.ctx, klog.Fields{
		"smtp.cmd":      "mail",
		"smtp.mailfrom": from,
	})
	u, err := uid.NewSnowflake(mailidRandSize)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to generate mail id"), nil)
		return errSMTPBase
	}
	id := u.Base32()
	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.id": id,
	})
	addr, err := gomail.ParseAddress(from)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp from addr"), nil)
		return errSMTPFromAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok || localPart == "" || domain == "" {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp from addr parts"), nil)
		return errSMTPFromAddr
	}
	// DMARC requires checking RFC5321.MailFrom identity and not RFC5321.HELO
	result, spfErr, err := s.checkSPF(ctx, domain, from)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed smtp from addr spf check"), klog.Fields{
			"smtp.from":       addr.Address,
			"smtp.spf.result": string(result),
			"smtp.spf.error":  spfErr.Error(),
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
	mailboxKeySeparator    = "."
	listSenderPolicyOwner  = "owner"
	listSenderPolicyMember = "member"
	listSenderPolicyUser   = "user"
	listMemberPolicyOwner  = "owner"
	listMemberPolicyUser   = "user"
)

func (s *smtpSession) Rcpt(to string) error {
	if s.from == "" {
		s.log.Warn(s.ctx, "Failed smtp command sequence", klog.Fields{
			"smtp.cmd": "to",
			"smtp.to":  to,
		})
		return errSMTPSeq
	}
	ctx := klog.WithFields(s.ctx, klog.Fields{
		"smtp.cmd":  "to",
		"smtp.id":   s.id,
		"smtp.from": s.from,
		"smtp.to":   to,
	})
	if s.rcptTo != "" {
		s.log.Warn(ctx, "Failed smtp rcpt count", klog.Fields{
			"smtp.rcptto": s.rcptTo,
		})
		return errSMTPRcptCount
	}
	addr, err := gomail.ParseAddress(to)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp to addr"), nil)
		return errSMTPRcptAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		s.log.Warn(ctx, "Failed to parse smtp to addr parts", nil)
		return errSMTPRcptAddr
	}
	listCreator, listname, ok := strings.Cut(localPart, mailboxKeySeparator)
	if !ok {
		s.log.Warn(ctx, "Failed to parse smtp to addr parts", nil)
		return errSMTPMailbox
	}
	isOrg := domain == s.service.orgdomain
	if domain != s.service.usrdomain && !isOrg {
		s.log.Warn(ctx, "Invalid smtp to domain", nil)
		return errSMTPSystem
	}

	var listCreatorID string
	if isOrg {
		creator, err := s.service.orgs.GetByName(ctx, listCreator)
		if err != nil {
			if errors.Is(err, db.ErrorNotFound{}) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "Owner org not found"), nil)
				return errSMTPMailbox
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get owner org by name"), nil)
			return errSMTPBase
		}
		listCreatorID = rank.ToOrgName(creator.OrgID)
	} else {
		creator, err := s.service.users.GetByUsername(ctx, listCreator)
		if err != nil {
			if errors.Is(err, db.ErrorNotFound{}) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "Owner user not found"), nil)
				return errSMTPMailbox
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get owner user by name"), nil)
			return errSMTPBase
		}
		listCreatorID = creator.Userid
	}

	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.list.owner": listCreatorID,
	})

	list, err := s.service.lists.GetList(ctx, listCreatorID, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			s.log.WarnErr(ctx, kerrors.WithMsg(err, "Mailbox not found"), nil)
			return errSMTPMailbox
		}
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get mailbox"), nil)
		return errSMTPBase
	}

	if list.Archive {
		s.log.Warn(ctx, "Mailbox is archived", klog.Fields{
			"smtp.list": list.ListID,
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
)

const (
	maxSubjectLength = 127
)

func (s *smtpSession) isAligned(a, b string) bool {
	return strings.HasSuffix(a, b) || strings.HasSuffix(b, a)
}

func (s *smtpSession) checkListPolicy(ctx context.Context, senderid string, msgid string) error {
	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.list.policy": s.senderPolicy,
	})
	switch s.senderPolicy {
	case listSenderPolicyOwner:
		if s.isOrg {
			if ok, err := gate.AuthMember(ctx, s.service.gate, senderid, s.rcptOwner); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth org member"), nil)
				return errSMTPBase
			} else if !ok {
				s.log.Warn(ctx, "Not allowed to send", nil)
				return errSMTPAuthSend
			}
		} else {
			if senderid != s.rcptOwner {
				s.log.Warn(ctx, "Not allowed to send", nil)
				return errSMTPAuthSend
			}
			if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"), nil)
				return errSMTPBase
			} else if !ok {
				s.log.Warn(ctx, "Not allowed to send", nil)
				return errSMTPAuthSend
			}
		}
	case listSenderPolicyMember:
		if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"), nil)
			return errSMTPBase
		} else if !ok {
			s.log.Warn(ctx, "Not allowed to send", nil)
			return errSMTPAuthSend
		}
		if _, err := s.service.lists.GetMember(ctx, s.rcptList, senderid); err != nil {
			if errors.Is(err, db.ErrorNotFound{}) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "List member not found"), nil)
				return errSMTPAuthSend
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get list member"), nil)
			return errSMTPBase
		}
	case listSenderPolicyUser:
		if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"), nil)
			return errSMTPBase
		} else if !ok {
			s.log.Warn(ctx, "Not allowed to send", nil)
			return errSMTPAuthSend
		}
	default:
		s.log.Warn(ctx, "Invalid mailbox sender policy", nil)
		return errSMTPMailboxConfig
	}
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	ctx := klog.WithFields(s.ctx, klog.Fields{
		"smtp.cmd":  "data",
		"smtp.id":   s.id,
		"smtp.from": s.from,
		"smtp.list": s.rcptList,
	})
	if s.from == "" || s.rcptTo == "" {
		s.log.Warn(ctx, "Failed smtp command sequence", nil)
		return errSMTPSeq
	}

	b := bytes.Buffer{}
	if _, err := io.Copy(&b, r); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to read smtp data"), nil)
		return errSMTPBaseExists
	}
	m, err := message.Read(bytes.NewReader(b.Bytes()))
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp data"), nil)
		return errMailBody
	}
	headers := emmail.Header{
		Header: m.Header,
	}

	msgid, err := headers.MessageID()
	if err != nil || msgid == "" {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail msgid"), nil)
		return errMailBody
	}

	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.msgid": msgid,
	})

	contentType, _, err := headers.ContentType()
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail content type"), nil)
		return errMailBody
	}

	fromAddrs, err := headers.AddressList(headerFrom)
	if err != nil || len(fromAddrs) == 0 {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail header from"), nil)
		return errMailBody
	}
	if len(fromAddrs) != 1 {
		s.log.Warn(ctx, "Invalid mail header from", nil)
		return errSPFAlignment
	}
	fromAddr := fromAddrs[0].Address
	localPart, fromAddrDomain, ok := strings.Cut(fromAddr, "@")
	if !ok || localPart == "" || fromAddrDomain == "" {
		s.log.Warn(ctx, "Failed to parse mail header from to parts", nil)
		return errMailBody
	}

	if !s.isAligned(s.fromDomain, fromAddrDomain) {
		s.log.Warn(ctx, "Failed spf alignment", klog.Fields{
			"smtp.sender.addr": fromAddr,
		})
		return errSPFAlignment
	}

	sender, err := s.service.users.GetByEmail(ctx, fromAddr)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			s.log.WarnErr(ctx, kerrors.WithMsg(err, "Sender user not found"), klog.Fields{
				"smtp.sender.addr": fromAddr,
			})
			return errSMTPAuthSend
		}
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get sender user by email"), nil)
		return errSMTPBase
	}

	ctx = klog.WithFields(ctx, klog.Fields{
		"smtp.sender": sender.Userid,
	})

	if err := s.checkListPolicy(ctx, sender.Userid, msgid); err != nil {
		return err
	}

	dmarcRec, dmarcErr := dmarc.LookupWithOptions(fromAddrDomain, &dmarc.LookupOptions{
		LookupTXT: func(domain string) ([]string, error) {
			return s.service.resolver.LookupTXT(ctx, domain)
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
			return s.service.resolver.LookupTXT(ctx, domain)
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
				s.log.Warn(ctx, "Failed spf alignment", klog.Fields{
					"smtp.dmarc.policy": string(dmarcRec.Policy),
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
	m.Header.Add(headerReceived, fmt.Sprintf("from %s (%s [%s]) by %s with %s id %s for %s; %s", s.helo, s.helo, s.srcip.String(), s.service.authdomain, "ESMTPS", s.id, s.rcptTo, time.Now().Round(0).UTC().Format(time.RFC1123Z)))

	mb := &bytes.Buffer{}
	if err := m.WriteTo(mb); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to write mail msg"), nil)
		return errSMTPBaseExists
	}

	j, err := kjson.Marshal(mailmsg{
		ListID: s.rcptList,
		MsgID:  msgid,
	})
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to encode list event to json"), nil)
		return errSMTPBaseExists
	}

	if msg, err := s.service.lists.GetMsg(ctx, s.rcptList, msgid); err != nil {
		if !errors.Is(err, db.ErrorNotFound{}) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get list msg"), nil)
			return errSMTPBaseExists
		}
	} else {
		// mail already exists for this list
		if !msg.Processed {
			if err := s.service.events.StreamPublish(ctx, s.service.opts.MailChannel, j); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish list event"), nil)
				return errSMTPBaseExists
			}
			if err := s.service.lists.MarkMsgProcessed(ctx, s.rcptList, msgid); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark list message processed"), nil)
			}
		}
		return nil
	}

	// must make a best effort to save the message and publish the event
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	if err := s.service.rcvMailDir.Subdir(s.rcptList).Put(ctx, s.service.encodeMsgid(msgid), contentType, int64(mb.Len()), nil, bytes.NewReader(mb.Bytes())); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to store mail msg"), nil)
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
	if err := s.service.lists.InsertMsg(ctx, msg); err != nil {
		if !errors.Is(err, db.ErrorUnique{}) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to add list msg"), nil)
			return errSMTPBaseExists
		}
		// Message has already been added for this list, but not guaranteed to be
		// sent yet, hence must continue with publishing the event and marking
		// message as processed.
	}
	if err := s.service.events.StreamPublish(ctx, s.service.opts.MailChannel, j); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish list event"), nil)
		return errSMTPBaseExists
	}
	if err := s.service.lists.MarkMsgProcessed(ctx, s.rcptList, msgid); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark list message processed"), nil)
	}
	s.log.Info(ctx, "Received mail", nil)
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
