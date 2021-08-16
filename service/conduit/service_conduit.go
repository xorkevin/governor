package conduit

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/chat/model"
	"xorkevin.dev/governor/service/db"
)

func (s *service) notifyChatEventGetMembers(kind string, m *model.ChatModel, userids []string) {
	members, err := s.repo.GetMembers(m.Chatid, 65536, 0)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("Failed to get chat members", map[string]string{
				"error":      err.Error(),
				"actiontype": "getchatmembers",
			})
		}
		members = nil
	}
	ids := make([]string, 0, len(members)+len(userids))
	for _, i := range members {
		ids = append(ids, i.Userid)
	}
	ids = append(ids, userids...)
	s.notifyChatEvent(kind, m, ids)
}

func (s *service) notifyChatEvent(kind string, m *model.ChatModel, userids []string) {
	if len(userids) == 0 {
		return
	}
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
	s.notifyChatEvent("chat.create", m, userids)
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
	s.notifyChatEventGetMembers("chat.update.settings", m, nil)
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
	s.notifyChatEventGetMembers("chat.update.members.add", m, nil)
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
	s.notifyChatEventGetMembers("chat.update.members.remove", m, userids)
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
	members, err := s.repo.GetMembers(chatid, 65536, 0)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members")
	}
	if err := s.repo.DeleteChatMembers(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
	}
	m.LastUpdated = time.Now().Round(0).UnixNano() / int64(time.Millisecond)
	userids := make([]string, 0, len(members))
	for _, i := range members {
		userids = append(userids, i.Userid)
	}
	s.notifyChatEvent("chat.delete", m, userids)
	if err := s.repo.DeleteChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat")
	}
	return nil
}
