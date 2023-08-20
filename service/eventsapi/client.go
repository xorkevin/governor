package eventsapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// CmdClient is a user cmd client
	CmdClient struct {
		gate         gate.Client
		log          *klog.LevelLogger
		term         governor.Term
		httpc        *governor.HTTPFetcher
		publishFlags publishFlags
	}

	publishFlags struct {
		subject string
		payload string
	}
)

func NewCmdClientCtx(inj governor.Injector) *CmdClient {
	return NewCmdClient(
		gate.GetCtxClient(inj),
	)
}

func NewCmdClient(g gate.Client) *CmdClient {
	return &CmdClient{
		gate: g,
	}
}

func (c *CmdClient) Register(inj governor.Injector, r governor.ConfigRegistrar, cr governor.CmdRegistrar) {
	cr.Register(governor.CmdDesc{
		Usage: "publish",
		Short: "publishes an event",
		Long:  "publishes an event",
		Flags: []governor.CmdFlag{
			{
				Long:     "subject",
				Short:    "s",
				Usage:    "subject",
				Required: true,
				Value:    &c.publishFlags.subject,
			},
			{
				Long:     "payload",
				Short:    "p",
				Usage:    "payload",
				Required: false,
				Value:    &c.publishFlags.payload,
			},
		},
	}, governor.CmdHandlerFunc(c.publishEvent))
}

func (c *CmdClient) Init(r governor.ClientConfigReader, log klog.Logger, term governor.Term, m governor.HTTPClient) error {
	c.log = klog.NewLevelLogger(log)
	c.term = term
	c.httpc = governor.NewHTTPFetcher(m)
	return nil
}

func (c *CmdClient) publishEvent(args []string) error {
	var payload bytes.Buffer
	if c.publishFlags.payload != "" {
		io.WriteString(&payload, c.publishFlags.payload)
	} else {
		if _, err := io.Copy(&payload, c.term.Stdin()); err != nil {
			return kerrors.WithMsg(err, "Failed reading event payload")
		}
	}
	var q url.Values
	q.Add("subject", c.publishFlags.subject)
	r, err := c.httpc.HTTPClient.Req(http.MethodPost, "/pubsub/publish?"+q.Encode(), &payload)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create events api requeust")
	}
	if err := c.gate.AddSysToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add systoken")
	}
	if _, err := c.httpc.DoNoContent(context.Background(), r); err != nil {
		return kerrors.WithMsg(err, "Failed publishing event")
	}
	c.log.Info(context.Background(), "Published event")
	return nil
}
