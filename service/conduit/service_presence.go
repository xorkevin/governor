package conduit

import (
	"encoding/json"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/ws"
)

const (
	locDM  = "dm"
	locGDM = "gdm"
)

func (s *service) PresenceHandler(topic string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": ws.PresenceChannelAll(s.wsopts.PresenceChannel),
		"group":   s.streamns + "_WORKER_PRESENCE",
	})
	userid := strings.TrimPrefix(topic, ws.PresenceChannelPrefix(s.wsopts.PresenceChannel))
	if userid == "" || len(userid) > lengthCapUserid {
		l.Error("Invalid userid", nil)
		return
	}
	req := &ws.PresenceEventProps{}
	if err := json.Unmarshal(msgdata, &req); err != nil {
		l.Error("Invalid presence event msg format", map[string]string{
			"error":  err.Error(),
			"userid": userid,
		})
		return
	}
	if !strings.HasPrefix(req.Location, s.channelns+".") {
		return
	}
	subloc := strings.TrimPrefix(req.Location, s.channelns+".")
	switch subloc {
	case locDM, locGDM:
	default:
		return
	}
	if err := s.kvpresence.Set(userid, subloc, 60); err != nil {
		l.Error("Failed to set presence", map[string]string{
			"error":  err.Error(),
			"userid": userid,
		})
		return
	}
}

func (s *service) getPresence(loc string, userids []string) ([]string, error) {
	if len(userids) == 0 {
		return nil, nil
	}

	kvres := make([]kvstore.Resulter, 0, len(userids))
	mget, err := s.kvpresence.Multi()
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to begin kv multi")
	}
	for _, i := range userids {
		kvres = append(kvres, mget.Get(i))
	}
	if err := mget.Exec(); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to exec kv multi")
	}

	res := make([]string, 0, len(kvres))
	for n, i := range kvres {
		k, err := i.Result()
		if err != nil {
			continue
		}
		if loc == "" || k == loc {
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

func (s *service) PresenceQueryHandler(topic string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": ws.ServiceChannelAll(s.wsopts.UserRcvChannelPrefix, s.opts.PresenceQueryChannel),
		"group":   s.streamns + "_PRESENCE_QUERY",
	})
	userid := strings.TrimPrefix(topic, ws.ServiceChannelPrefix(s.wsopts.UserRcvChannelPrefix, s.opts.PresenceQueryChannel))
	req := reqGetPresence{}
	if err := json.Unmarshal(msgdata, &req); err != nil {
		return
	}
	req.Userid = userid
	if err := req.valid(); err != nil {
		return
	}

	m, err := s.friends.GetFriendsByID(req.Userid, req.Userids)
	if err != nil {
		l.Error("Failed to get user friends", map[string]string{
			"error": err.Error(),
		})
		return
	}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid2)
	}

	if len(userids) == 0 {
		b, err := json.Marshal(resPresence{
			Userids: userids,
		})
		if err != nil {
			l.Error("Failed to marshal presence res json", map[string]string{
				"error": err.Error(),
			})
			return
		}
		if err := s.events.Publish(ws.UserChannel(s.wsopts.UserSendChannelPrefix, req.Userid, s.channelns+".presence"), b); err != nil {
			l.Error("Failed to publish presence res event", map[string]string{
				"error": err.Error(),
			})
			return
		}
		return
	}

	res, err := s.getPresence("", userids)
	if err != nil {
		l.Error("Failed to get presence", map[string]string{
			"error": err.Error(),
		})
		return
	}

	b, err := json.Marshal(resPresence{
		Userids: res,
	})
	if err != nil {
		l.Error("Failed to marshal presence res json", map[string]string{
			"error": err.Error(),
		})
		return
	}
	if err := s.events.Publish(ws.UserChannel(s.wsopts.UserSendChannelPrefix, req.Userid, s.channelns+".presence"), b); err != nil {
		l.Error("Failed to publish presence res event", map[string]string{
			"error": err.Error(),
		})
		return
	}
}
