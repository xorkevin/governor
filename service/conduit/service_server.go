package conduit

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/server/model"
	"xorkevin.dev/governor/service/db"
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

func (s *service) CreateServer(serverid string, name, desc string, theme string) (*resServer, error) {
	m := s.servers.New(serverid, name, desc, theme)
	if err := s.servers.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusConflict,
				Message: "Server already created",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to create server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetServer(serverid string) (*resServer, error) {
	m, err := s.servers.GetServer(serverid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) UpdateServer(serverid string, name, desc string, theme string) error {
	m, err := s.servers.GetServer(serverid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get server")
	}
	m.Name = name
	m.Desc = desc
	m.Theme = theme
	if err := s.servers.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update server")
	}
	// TODO publish server update event
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

func (s *service) CreateChannel(serverid, channelid string, name, desc string, theme string) (*resChannel, error) {
	if _, err := s.servers.GetServer(serverid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get server")
	}
	m, err := s.servers.NewChannel(serverid, channelid, name, desc, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create channel")
	}
	if err := s.servers.InsertChannel(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusConflict,
				Message: "Channel already created",
			}), governor.ErrOptInner(err))
		}
	}
	// TODO publish chat create event
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

func (s *service) getServerChannel(serverid, channelid string) (*model.ChannelModel, error) {
	if _, err := s.servers.GetServer(serverid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get server")
	}
	m, err := s.servers.GetChannel(serverid, channelid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Channel not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get channel")
	}
	return m, nil
}

func (s *service) GetChannel(serverid, channelid string) (*resChannel, error) {
	m, err := s.getServerChannel(serverid, channelid)
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

func (s *service) GetChannels(serverid string, prefix string, limit, offset int) (*resChannels, error) {
	if _, err := s.servers.GetServer(serverid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get server")
	}
	m, err := s.servers.GetChannels(serverid, prefix, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get channels")
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

func (s *service) UpdateChannel(serverid, channelid string, name, desc string, theme string) error {
	m, err := s.getServerChannel(serverid, channelid)
	if err != nil {
		return err
	}
	m.Name = name
	m.Desc = desc
	m.Theme = theme
	if err := s.servers.UpdateChannel(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update channel")
	}
	// TODO publish chat update event
	return nil
}

func (s *service) DeleteChannel(serverid, channelid string) error {
	m, err := s.getServerChannel(serverid, channelid)
	if err != nil {
		return err
	}
	if err := s.msgs.DeleteChatMsgs(m.Chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete channel messages")
	}
	if err := s.servers.DeleteChannels(serverid, []string{channelid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete channel")
	}
	// TODO publish chat delete event
	return nil
}

func (s *service) CreateChannelMsg(serverid, channelid string, userid string, kind string, value string) (*resMsg, error) {
	ch, err := s.getServerChannel(serverid, channelid)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.New(ch.Chatid, userid, kind, value)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new server msg")
	}
	if err := s.msgs.Insert(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to send new server msg")
	}
	res := resMsg{
		Chatid: m.Chatid,
		Msgid:  m.Msgid,
		Userid: m.Userid,
		Timems: m.Timems,
		Kind:   m.Kind,
		Value:  m.Value,
	}
	// TODO publish chat message event
	return &res, nil
}

func (s *service) GetChannelMsgs(serverid, channelid string, userid string, kind string, before string, limit int) (*resMsgs, error) {
	ch, err := s.getServerChannel(serverid, channelid)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.GetMsgs(ch.Chatid, kind, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get server chat msgs")
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

func (s *service) DelServerMsg(serverid, channelid string, msgid string) error {
	ch, err := s.getServerChannel(serverid, channelid)
	if err != nil {
		return err
	}
	if err := s.msgs.DeleteMsgs(ch.Chatid, []string{msgid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete server chat msg")
	}
	// TODO: emit msg delete event
	return nil
}
