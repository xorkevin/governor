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
		cli          governor.CLI
		http         governor.HTTPClient
		url          string
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

func (c *CmdClient) Init(gc governor.ClientConfig, r governor.ConfigValueReader, log klog.Logger, cli governor.CLI, m governor.HTTPClient) error {
	c.log = klog.NewLevelLogger(log)
	c.cli = cli
	c.http = m
	c.url = r.URL()
	return nil
}

func (c *CmdClient) publishEvent(args []string) error {
	var payload bytes.Buffer
	if c.publishFlags.payload != "" {
		if _, err := io.WriteString(&payload, c.publishFlags.payload); err != nil {
			return kerrors.WithMsg(err, "Failed creating event payload")
		}
	} else {
		if _, err := io.Copy(&payload, c.cli.Stdin()); err != nil {
			return kerrors.WithMsg(err, "Failed reading event payload")
		}
	}
	u, err := url.Parse(c.url + "/pubsub/publish")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create events api url")
	}
	q := u.Query()
	q.Add("subject", c.publishFlags.subject)
	u.RawQuery = q.Encode()
	r, err := c.http.NewRequest(http.MethodPost, u.String(), &payload)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create events api requeust")
	}
	if err := c.gate.AddSysToken(r); err != nil {
		return kerrors.WithMsg(err, "Failed to add systoken")
	}
	if _, err := c.http.DoRequestNoContent(context.Background(), r); err != nil {
		return kerrors.WithMsg(err, "Failed publishing event")
	}
	c.log.Info(context.Background(), "Published event", nil)
	return nil
}
