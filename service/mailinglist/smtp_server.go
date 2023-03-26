package mailinglist

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	gomail "net/mail"
	"net/netip"
	"strings"
	"sync/atomic"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/emersion/go-message"
	emmail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-msgauth/authres"
	"github.com/emersion/go-msgauth/dkim"
	"github.com/emersion/go-msgauth/dmarc"
	"github.com/emersion/go-smtp"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
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
	service  *Service
	instance string
	log      *klog.LevelLogger
	reqcount *atomic.Uint32
}

func (s *smtpBackend) lreqID() string {
	return s.instance + "-" + uid.ReqID(s.reqcount.Add(1))
}

func (s *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ctx := klog.CtxWithAttrs(context.Background(),
		klog.AString("smtp.cmd", "helo"),
	)
	addrport, err := netip.ParseAddrPort(c.Conn().RemoteAddr().String())
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp remote addr"),
			klog.AString("smtp.remoteaddr", c.Conn().RemoteAddr().String()),
		)
		return nil, errSMTPConn
	}
	addr := addrport.Addr()
	hostname := c.Hostname()
	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("smtp.session.ip", addr.String()),
		klog.AString("smtp.session.helo", hostname),
	)
	return &smtpSession{
		service: s.service,
		be:      s,
		log:     klog.NewLevelLogger(s.log.Logger.Sublogger("session")),
		ctx:     ctx,
		srcip:   addr,
		helo:    hostname,
	}, nil
}

type smtpSession struct {
	service      *Service
	be           *smtpBackend
	log          *klog.LevelLogger
	ctx          context.Context
	srcip        netip.Addr
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
	result, spfErr := spf.CheckHostWithSender(net.IP(s.srcip.AsSlice()), domain, from, spf.WithContext(ctx), spf.WithResolver(s.service.resolver))
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

func (s *smtpSession) AuthPlain(username, password string) error {
	return smtp.ErrAuthUnsupported
}

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	ctx := klog.CtxWithAttrs(s.ctx,
		klog.AString("smtp.cmd", "mail"),
		klog.AString("smtp.mailfrom", from),
	)
	id := s.be.lreqID()
	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("reqid", id),
	)
	addr, err := gomail.ParseAddress(from)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp from addr"))
		return errSMTPFromAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok || localPart == "" || domain == "" {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp from addr parts"))
		return errSMTPFromAddr
	}
	// DMARC requires checking RFC5321.MailFrom identity and not RFC5321.HELO
	result, spfErr, err := s.checkSPF(ctx, domain, from)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed smtp from addr spf check"),
			klog.AString("smtp.from", addr.Address),
			klog.AString("smtp.spf.result", string(result)),
			klog.AString("smtp.spf.error", spfErr.Error()),
		)
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
		s.log.Warn(s.ctx, "Failed smtp command sequence",
			klog.AString("smtp.cmd", "to"),
			klog.AString("smtp.to", to),
		)
		return errSMTPSeq
	}
	ctx := klog.CtxWithAttrs(s.ctx,
		klog.AString("smtp.cmd", "to"),
		klog.AString("reqid", s.id),
		klog.AString("smtp.from", s.from),
		klog.AString("smtp.to", to),
	)
	if s.rcptTo != "" {
		s.log.Warn(ctx, "Failed smtp rcpt count",
			klog.AString("smtp.rcptto", s.rcptTo),
		)
		return errSMTPRcptCount
	}
	addr, err := gomail.ParseAddress(to)
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp to addr"))
		return errSMTPRcptAddr
	}
	localPart, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		s.log.Warn(ctx, "Failed to parse smtp to addr parts")
		return errSMTPRcptAddr
	}
	listCreator, listname, ok := strings.Cut(localPart, mailboxKeySeparator)
	if !ok {
		s.log.Warn(ctx, "Failed to parse smtp to addr parts")
		return errSMTPMailbox
	}
	isOrg := domain == s.service.orgdomain
	if domain != s.service.usrdomain && !isOrg {
		s.log.Warn(ctx, "Invalid smtp to domain")
		return errSMTPSystem
	}

	var listCreatorID string
	if isOrg {
		creator, err := s.service.orgs.GetByName(ctx, listCreator)
		if err != nil {
			if errors.Is(err, org.ErrorNotFound) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "Owner org not found"))
				return errSMTPMailbox
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get owner org by name"))
			return errSMTPBase
		}
		listCreatorID = rank.ToOrgName(creator.OrgID)
	} else {
		creator, err := s.service.users.GetByUsername(ctx, listCreator)
		if err != nil {
			if errors.Is(err, user.ErrorNotFound) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "Owner user not found"))
				return errSMTPMailbox
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get owner user by name"))
			return errSMTPBase
		}
		listCreatorID = creator.Userid
	}

	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("list.owner", listCreatorID),
	)

	list, err := s.service.lists.GetList(ctx, listCreatorID, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			s.log.WarnErr(ctx, kerrors.WithMsg(err, "Mailbox not found"))
			return errSMTPMailbox
		}
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get mailbox"))
		return errSMTPBase
	}

	if list.Archive {
		s.log.Warn(ctx, "Mailbox is archived",
			klog.AString("list", list.ListID),
		)
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
	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("list.policy", s.senderPolicy),
	)
	switch s.senderPolicy {
	case listSenderPolicyOwner:
		if s.isOrg {
			if ok, err := gate.AuthMember(ctx, s.service.gate, senderid, s.rcptOwner); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth org member"))
				return errSMTPBase
			} else if !ok {
				s.log.Warn(ctx, "Not allowed to send")
				return errSMTPAuthSend
			}
		} else {
			if senderid != s.rcptOwner {
				s.log.Warn(ctx, "Not allowed to send")
				return errSMTPAuthSend
			}
			if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"))
				return errSMTPBase
			} else if !ok {
				s.log.Warn(ctx, "Not allowed to send")
				return errSMTPAuthSend
			}
		}
	case listSenderPolicyMember:
		if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"))
			return errSMTPBase
		} else if !ok {
			s.log.Warn(ctx, "Not allowed to send")
			return errSMTPAuthSend
		}
		if _, err := s.service.lists.GetMember(ctx, s.rcptList, senderid); err != nil {
			if errors.Is(err, db.ErrorNotFound) {
				s.log.WarnErr(ctx, kerrors.WithMsg(err, "List member not found"))
				return errSMTPAuthSend
			}
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get list member"))
			return errSMTPBase
		}
	case listSenderPolicyUser:
		if ok, err := gate.AuthUser(ctx, s.service.gate, senderid); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to auth user"))
			return errSMTPBase
		} else if !ok {
			s.log.Warn(ctx, "Not allowed to send")
			return errSMTPAuthSend
		}
	default:
		s.log.Warn(ctx, "Invalid mailbox sender policy")
		return errSMTPMailboxConfig
	}
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	ctx := klog.CtxWithAttrs(s.ctx,
		klog.AString("smtp.cmd", "data"),
		klog.AString("reqid", s.id),
		klog.AString("smtp.from", s.from),
		klog.AString("smtp.list", s.rcptList),
	)
	if s.from == "" || s.rcptTo == "" {
		s.log.Warn(ctx, "Failed smtp command sequence")
		return errSMTPSeq
	}

	var b bytes.Buffer
	if _, err := io.Copy(&b, r); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to read smtp data"))
		return errSMTPBaseExists
	}
	m, err := message.Read(bytes.NewReader(b.Bytes()))
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse smtp data"))
		return errMailBody
	}
	headers := emmail.Header{
		Header: m.Header,
	}

	msgid, err := headers.MessageID()
	if err != nil || msgid == "" {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail msgid"))
		return errMailBody
	}

	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("smtp.msgid", msgid),
	)

	contentType, _, err := headers.ContentType()
	if err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail content type"))
		return errMailBody
	}

	fromAddrs, err := headers.AddressList(headerFrom)
	if err != nil || len(fromAddrs) == 0 {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to parse mail header from"))
		return errMailBody
	}
	if len(fromAddrs) != 1 {
		s.log.Warn(ctx, "Invalid mail header from")
		return errSPFAlignment
	}
	fromAddr := fromAddrs[0].Address
	localPart, fromAddrDomain, ok := strings.Cut(fromAddr, "@")
	if !ok || localPart == "" || fromAddrDomain == "" {
		s.log.Warn(ctx, "Failed to parse mail header from to parts")
		return errMailBody
	}

	if !s.isAligned(s.fromDomain, fromAddrDomain) {
		s.log.Warn(ctx, "Failed spf alignment",
			klog.AString("smtp.sender.addr", fromAddr),
		)
		return errSPFAlignment
	}

	sender, err := s.service.users.GetByEmail(ctx, fromAddr)
	if err != nil {
		if errors.Is(err, user.ErrorNotFound) {
			s.log.WarnErr(ctx, kerrors.WithMsg(err, "Sender user not found"),
				klog.AString("smtp.sender.addr", fromAddr),
			)
			return errSMTPAuthSend
		}
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get sender user by email"))
		return errSMTPBase
	}

	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("sender", sender.Userid),
	)

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
				s.log.Warn(ctx, "Failed spf alignment",
					klog.AString("smtp.dmarc.policy", string(dmarcRec.Policy)),
				)
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

	var mb bytes.Buffer
	if err := m.WriteTo(&mb); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to write mail msg"))
		return errSMTPBaseExists
	}

	j, err := encodeListEventMail(mailProps{
		ListID: s.rcptList,
		MsgID:  msgid,
	})
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to encode list event to json"))
		return errSMTPBaseExists
	}

	if msg, err := s.service.lists.GetMsg(ctx, s.rcptList, msgid); err != nil {
		if !errors.Is(err, db.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get list msg"))
			return errSMTPBaseExists
		}
	} else {
		// mail already exists for this list
		if !msg.Processed {
			if err := s.service.events.Publish(ctx, events.NewMsgs(s.service.streammail, s.rcptList, j)...); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish list event"))
				return errSMTPBaseExists
			}
			if err := s.service.lists.MarkMsgProcessed(ctx, s.rcptList, msgid); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark list message processed"))
			}
		}
		return nil
	}

	// must make a best effort to save the message and publish the event
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.service.rcvMailDir.Subdir(s.rcptList).Put(ctx, s.service.encodeMsgid(msgid), contentType, int64(mb.Len()), nil, bytes.NewReader(mb.Bytes())); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to store mail msg"))
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
		if !errors.Is(err, db.ErrorUnique) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to add list msg"))
			return errSMTPBaseExists
		}
		// Message has already been added for this list, but not guaranteed to be
		// sent yet, hence must continue with publishing the event and marking
		// message as processed.
	}
	if err := s.service.events.Publish(ctx, events.NewMsgs(s.service.streammail, s.rcptList, j)...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish list event"))
		return errSMTPBaseExists
	}
	if err := s.service.lists.MarkMsgProcessed(ctx, s.rcptList, msgid); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to mark list message processed"))
	}
	s.log.Info(ctx, "Received mail")
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
