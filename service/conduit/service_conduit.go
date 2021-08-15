package conduit

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

func (s *service) notifyChatEvent(kind string, chatid string, userids []string) {
}

type (
	resChat struct {
		ChatID       string `json:"chatid"`
		Kind         string `json:"kind"`
		Name         string `json:"name"`
		Theme        string `json:"theme"`
		LastUpdated  int64  `json:"last_updated"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *service) CreateChat(kind string, name string, theme string) (*resChat, error) {
	m, err := s.repo.NewChat(kind, name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat id")
	}
	if err := s.repo.InsertChat(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat")
	}
	s.notifyChatEvent("chat.create", m.Chatid, nil)
	return &resChat{
		ChatID:       m.Chatid,
		Kind:         m.Chatid,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) CreateChatWithUsers(kind string, name string, theme string, userids []string) (*resChat, error) {
	m, err := s.repo.NewChat(kind, name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat id")
	}
	members := s.repo.AddMembers(m, userids)
	if err := s.repo.InsertChat(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat")
	}
	if err := s.repo.InsertMembers(members); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to add chat members")
	}
	s.notifyChatEvent("chat.create", m.Chatid, nil)
	return &resChat{
		ChatID:       m.Chatid,
		Kind:         m.Chatid,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) UpdateChat(chatid string, name string, theme string) error {
	m, err := s.repo.GetChat(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Chat not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get chat")
	}
	m.Name = name
	m.Theme = theme
	m.LastUpdated = time.Now().Round(0).UnixNano() / int64(time.Millisecond)
	if err := s.repo.UpdateChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, m.LastUpdated); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	s.notifyChatEvent("chat.update", m.Chatid, nil)
	return nil
}

func (s *service) AddChatMembers(chatid string, userids []string) error {
	m, err := s.repo.GetChat(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Chat not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get chat")
	}
	members := s.repo.AddMembers(m, userids)
	if err := s.repo.UpdateChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, m.LastUpdated); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.InsertMembers(members); err != nil {
		return governor.ErrWithMsg(err, "Failed to add chat members")
	}
	s.notifyChatEvent("chat.update", m.Chatid, nil)
	return nil
}

func (s *service) RemoveChatMembers(chatid string, userids []string) error {
	m, err := s.repo.GetChat(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Chat not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get chat")
	}
	m.LastUpdated = time.Now().Round(0).UnixNano() / int64(time.Millisecond)
	if err := s.repo.UpdateChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, m.LastUpdated); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.DeleteMembers(m.Chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove chat members")
	}
	s.notifyChatEvent("chat.update", m.Chatid, userids)
	return nil
}

func (s *service) DeleteChat(chatid string) error {
	m, err := s.repo.GetChat(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Chat not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get chat")
	}
	if err := s.repo.DeleteChatMsgs(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat messages")
	}
	if err := s.repo.DeleteChatMembers(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
	}
	if err := s.repo.DeleteChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat")
	}
	s.notifyChatEvent("chat.delete", m.Chatid, nil)
	return nil
}
