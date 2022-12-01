package conduit

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/servermodel"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

type (
	resServer struct {
		ServerID     string `json:"serverid"`
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		Theme        string `json:"theme"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *Service) createServer(ctx context.Context, serverid string, name, desc string, theme string) (*resServer, error) {
	m := s.servers.New(serverid, name, desc, theme)
	if err := s.servers.Insert(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique) {
			return nil, governor.ErrWithRes(err, http.StatusConflict, "", "Server already created")
		}
		return nil, kerrors.WithMsg(err, "Failed to create server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *Service) getServer(ctx context.Context, serverid string) (*resServer, error) {
	m, err := s.servers.GetServer(ctx, serverid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Server not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *Service) updateServer(ctx context.Context, serverid string, name, desc string, theme string) error {
	m, err := s.servers.GetServer(ctx, serverid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Server not found")
		}
		return kerrors.WithMsg(err, "Failed to get server")
	}
	m.Name = name
	m.Desc = desc
	m.Theme = theme
	if err := s.servers.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update server")
	}
	// TODO publish server settings update event
	return nil
}

type (
	resChannel struct {
		ServerID     string `json:"serverid"`
		ChannelID    string `json:"channelid"`
		Chatid       string `json:"chatid"`
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		Theme        string `json:"theme"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *Service) createChannel(ctx context.Context, serverid, channelid string, name, desc string, theme string) (*resChannel, error) {
	if _, err := s.servers.GetServer(ctx, serverid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Server not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get server")
	}
	m, err := s.servers.NewChannel(serverid, channelid, name, desc, theme)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create channel")
	}
	if err := s.servers.InsertChannel(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique) {
			return nil, governor.ErrWithRes(err, http.StatusConflict, "", "Channel already created")
		}
	}
	// TODO publish channel create event
	return &resChannel{
		ServerID:     m.ServerID,
		ChannelID:    m.ChannelID,
		Chatid:       m.Chatid,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *Service) getServerChannel(ctx context.Context, serverid, channelid string) (*servermodel.ChannelModel, error) {
	if _, err := s.servers.GetServer(ctx, serverid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Server not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get server")
	}
	m, err := s.servers.GetChannel(ctx, serverid, channelid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Channel not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get channel")
	}
	return m, nil
}

func (s *Service) getChannel(ctx context.Context, serverid, channelid string) (*resChannel, error) {
	m, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return nil, err
	}
	return &resChannel{
		ServerID:     m.ServerID,
		ChannelID:    m.ChannelID,
		Chatid:       m.Chatid,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

type (
	resChannels struct {
		Channels []resChannel `json:"channels"`
	}
)

func (s *Service) getChannels(ctx context.Context, serverid string, prefix string, limit, offset int) (*resChannels, error) {
	if _, err := s.servers.GetServer(ctx, serverid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Server not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get server")
	}
	m, err := s.servers.GetChannels(ctx, serverid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get channels")
	}
	res := make([]resChannel, 0, len(m))
	for _, i := range m {
		res = append(res, resChannel{
			ServerID:     i.ServerID,
			ChannelID:    i.ChannelID,
			Chatid:       i.Chatid,
			Name:         i.Name,
			Desc:         i.Desc,
			Theme:        i.Theme,
			CreationTime: i.CreationTime,
		})
	}
	return &resChannels{
		Channels: res,
	}, nil
}

func (s *Service) updateChannel(ctx context.Context, serverid, channelid string, name, desc string, theme string) error {
	m, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return err
	}
	m.Name = name
	m.Desc = desc
	m.Theme = theme
	if err := s.servers.UpdateChannelProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update channel")
	}
	// TODO publish channel settings update event
	return nil
}

func (s *Service) deleteChannel(ctx context.Context, serverid, channelid string) error {
	m, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return err
	}
	if err := s.msgs.DeleteChatMsgs(ctx, m.Chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete channel messages")
	}
	if err := s.servers.DeleteChannels(ctx, serverid, []string{channelid}); err != nil {
		return kerrors.WithMsg(err, "Failed to delete channel")
	}
	// TODO publish chat delete event
	return nil
}

func (s *Service) createChannelMsg(ctx context.Context, serverid, channelid string, userid string, kind string, value string) (*resMsg, error) {
	ch, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.New(ch.Chatid, userid, kind, value)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new server msg")
	}
	if err := s.msgs.Insert(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to send new server msg")
	}
	res := resMsg{
		Chatid: m.Chatid,
		Msgid:  m.Msgid,
		Userid: m.Userid,
		Timems: m.Timems,
		Kind:   m.Kind,
		Value:  m.Value,
	}
	// TODO publish channel message event
	return &res, nil
}

func (s *Service) getChannelMsgs(ctx context.Context, serverid, channelid string, kind string, before string, limit int) (*resMsgs, error) {
	ch, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.GetMsgs(ctx, ch.Chatid, kind, before, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get server chat msgs")
	}
	res := make([]resMsg, 0, len(m))
	for _, i := range m {
		res = append(res, resMsg{
			Chatid: i.Chatid,
			Msgid:  i.Msgid,
			Userid: i.Userid,
			Timems: i.Timems,
			Kind:   i.Kind,
			Value:  i.Value,
		})
	}
	return &resMsgs{
		Msgs: res,
	}, nil
}

func (s *Service) deleteChannelMsg(ctx context.Context, serverid, channelid string, msgid string) error {
	ch, err := s.getServerChannel(ctx, serverid, channelid)
	if err != nil {
		return err
	}
	if err := s.msgs.EraseMsgs(ctx, ch.Chatid, []string{msgid}); err != nil {
		return kerrors.WithMsg(err, "Failed to delete server chat msg")
	}
	// TODO: publish msg delete event
	return nil
}
