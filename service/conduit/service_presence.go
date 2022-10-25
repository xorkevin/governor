package conduit

import (
	"context"
	"errors"
	"strings"

	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/ws"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
)

const (
	locDM  = "dm"
	locGDM = "gdm"
)

func (s *Service) presenceHandler(ctx context.Context, props ws.PresenceEventProps) error {
	subloc := strings.TrimPrefix(props.Location, s.channelns+".")
	switch subloc {
	// TODO handle server and channel presence
	case locDM, locGDM:
	default:
		return nil
	}
	if err := s.kvpresence.Set(ctx, props.Userid, subloc, 60); err != nil {
		return kerrors.WithMsg(err, "Failed to set presence")
	}
	return nil
}

func (s *Service) getPresence(ctx context.Context, loc string, userids []string) ([]string, error) {
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
			if !errors.Is(err, kvstore.ErrorNotFound) {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get presence result"), nil)
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

func (s *Service) presenceQueryHandler(ctx context.Context, topic string, userid string, msgdata []byte) error {
	var req reqGetPresence
	if err := kjson.Unmarshal(msgdata, &req); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Invalid get presence request"), nil)
		return nil
	}
	req.Userid = userid
	if err := req.valid(); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Invalid get presence request"), nil)
		return nil
	}

	m, err := s.friends.GetFriendsByID(ctx, req.Userid, req.Userids)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get user friends")
	}

	if len(m) == 0 {
		if err := s.ws.Publish(ctx, req.Userid, s.opts.PresenceQueryChannel, resPresence{
			Userids: []string{},
		}); err != nil {
			return kerrors.WithMsg(err, "Failed to publish presence res event")
		}
	}

	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid2)
	}

	res, err := s.getPresence(ctx, ">", userids)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get presence")
	}

	if err := s.ws.Publish(ctx, req.Userid, s.opts.PresenceQueryChannel, resPresence{
		Userids: res,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to publish presence res event")
	}
	return nil
}
