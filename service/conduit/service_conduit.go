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

const (
	chatMemberAmountCap = 255
)

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
	if members, err := s.repo.GetChatMembers(chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members")
	} else if len(members) != 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Chat member already added",
		}), governor.ErrOptInner(err))
	}
	if count, err := s.repo.GetMembersCount(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members count")
	} else if count+len(userids) > chatMemberAmountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "May not have more than 255 chat members",
		}), governor.ErrOptInner(err))
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
	if members, err := s.repo.GetChatMembers(chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members")
	} else if len(members) != len(userids) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusNotFound,
			Message: "Chat member does not exist",
		}), governor.ErrOptInner(err))
	}
	if count, err := s.repo.GetMembersCount(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members count")
	} else if count-len(userids) < 1 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "May not leave chat as the last member",
		}), governor.ErrOptInner(err))
	}

	m.LastUpdated = time.Now().Round(0).UnixNano() / int64(time.Millisecond)
	if err := s.repo.UpdateChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.DeleteMembers(m.Chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove chat members")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, m.LastUpdated); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
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

func (s *service) GetUserChats(userid string, chatids []string) ([]model.MemberModel, error) {
	m, err := s.repo.GetUserChats(userid, chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get user chats")
	}
	return m, nil
}

func (s *service) GetChats(chatids []string) ([]resChat, error) {
	m, err := s.repo.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chats")
	}
	chats := make([]resChat, 0, len(m))
	for _, i := range m {
		chats = append(chats, resChat{
			ChatID:       i.Chatid,
			Kind:         i.Kind,
			Name:         i.Name,
			Theme:        i.Theme,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
		})
	}
	return chats, nil
}

func (s *service) GetLatestChatsByKind(kind string, userid string, before int64, limit int) ([]resChat, error) {
	var members []model.MemberModel
	if before == 0 {
		var err error
		members, err = s.repo.GetLatestChatsByKind(kind, userid, limit, 0)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to get latest chats")
		}
	} else {
		var err error
		members, err = s.repo.GetLatestChatsBeforeByKind(kind, userid, before, limit)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to get latest chats")
		}
	}
	chatids := make([]string, 0, len(members))
	for _, i := range members {
		chatids = append(chatids, i.Chatid)
	}
	return s.GetChats(chatids)
}
