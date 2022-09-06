package conduit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/ws"
	"xorkevin.dev/kerrors"
)

const (
	locDM  = "dm"
	locGDM = "gdm"
)

func (s *service) presenceHandler(ctx context.Context, props ws.PresenceEventProps) {
	l := s.logger.WithData(map[string]string{
		"agent":    "subscriber",
		"location": props.Location,
		"group":    s.streamns + "_WORKER_PRESENCE",
		"userid":   props.Userid,
	})
	subloc := strings.TrimPrefix(props.Location, s.channelns+".")
	switch subloc {
	// TODO handle server and channel presence
	case locDM, locGDM:
	default:
		return
	}
	if err := s.kvpresence.Set(ctx, props.Userid, subloc, 60); err != nil {
		l.Error("Failed to set presence", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_set_presence",
		})
		return
	}
}

func (s *service) getPresence(ctx context.Context, l governor.Logger, loc string, userids []string) ([]string, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	kvres := make([]kvstore.Resulter, 0, len(userids))
	mget, err := s.kvpresence.Multi(ctx)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to begin kv multi")
	}
	for _, i := range userids {
		kvres = append(kvres, mget.Get(ctx, i))
	}
	if err := mget.Exec(ctx); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to exec kv multi")
	}

	res := make([]string, 0, len(kvres))
	for n, i := range kvres {
		k, err := i.Result()
		if err != nil {
			if !errors.Is(err, kvstore.ErrNotFound{}) {
				l.Error("Failed to get presence result", map[string]string{
					"error":      err.Error(),
					"actiontype": "conduit_get_presence_result",
				})
			}
			continue
		}
		// right angle means get all locations
		if loc == ">" || k == loc {
			res = append(res, userids[n])
		}
	}
	return res, nil
}

type (
	resPresence struct {
		Userids []string `json:"userids"`
	}
)

func (s *service) presenceQueryHandler(ctx context.Context, topic string, userid string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": s.opts.PresenceQueryChannel,
		"group":   s.streamns + "_PRESENCE_QUERY",
		"userid":  userid,
	})
	var req reqGetPresence
	if err := json.Unmarshal(msgdata, &req); err != nil {
		l.Warn("Invalid get presence request", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_decode_presence_req",
		})
		return
	}
	req.Userid = userid
	if err := req.valid(); err != nil {
		l.Warn("Invalid get presence request", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_validate_presence_req",
		})
		return
	}

	m, err := s.friends.GetFriendsByID(ctx, req.Userid, req.Userids)
	if err != nil {
		l.Error("Failed to get user friends", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_get_presence_friends",
		})
		return
	}

	if len(m) == 0 {
		if err := s.ws.Publish(ctx, req.Userid, s.opts.PresenceQueryChannel, resPresence{
			Userids: []string{},
		}); err != nil {
			l.Error("Failed to publish presence res event", map[string]string{
				"error":      err.Error(),
				"actiontype": "conduit_publish_presence_res",
			})
			return
		}
		return
	}

	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid2)
	}

	res, err := s.getPresence(ctx, l, "", userids)
	if err != nil {
		l.Error("Failed to get presence", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_get_presence",
		})
		return
	}

	if err := s.ws.Publish(ctx, req.Userid, s.opts.PresenceQueryChannel, resPresence{
		Userids: res,
	}); err != nil {
		l.Error("Failed to publish presence res event", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_publish_presence_res",
		})
		return
	}
}
