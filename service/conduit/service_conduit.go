package conduit

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/chat/model"
	"xorkevin.dev/governor/service/db"
)

func (s *service) notifyChatEventGetMembers(kind string, chatid string, userids []string) {
	members, err := s.repo.GetMembers(chatid, chatMemberAmountCap*2, 0)
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
	s.notifyChatEvent(kind, chatid, ids)
}

func (s *service) notifyChatEvent(kind string, chatid string, userids []string) {
	if len(userids) == 0 {
		return
	}
}

type (
	resChat struct {
		ChatID       string   `json:"chatid"`
		Kind         string   `json:"kind"`
		Name         string   `json:"name"`
		Theme        string   `json:"theme"`
		LastUpdated  int64    `json:"last_updated"`
		CreationTime int64    `json:"creation_time"`
		Members      []string `json:"members"`
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
		Members:      []string{},
	}, nil
}

func (s *service) checkUsersExist(userids []string) error {
	ids, err := s.users.CheckUsersExist(userids)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to users exist check")
	}
	if len(ids) != len(userids) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "User does not exist",
		}))
	}
	return nil
}

func (s *service) CreateChatWithUsers(kind string, name string, theme string, userids []string) (*resChat, error) {
	if err := s.checkUsersExist(userids); err != nil {
		return nil, err
	}

	m, err := s.repo.NewChat(kind, name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat id")
	}
	members, _ := s.repo.AddMembers(m, userids)
	if err := s.repo.InsertChat(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new chat")
	}
	if err := s.repo.InsertMembers(members); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to add chat members")
	}
	s.notifyChatEvent("chat.create", m.Chatid, userids)
	return &resChat{
		ChatID:       m.Chatid,
		Kind:         m.Kind,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
		Members:      userids,
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
	if err := s.repo.UpdateChat(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, time.Now().Round(0).UnixMilli()); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	s.notifyChatEventGetMembers("chat.update.settings", chatid, nil)
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

	if err := s.checkUsersExist(userids); err != nil {
		return err
	}

	members, now := s.repo.AddMembers(m, userids)
	if err := s.repo.InsertMembers(members); err != nil {
		return governor.ErrWithMsg(err, "Failed to add chat members")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, now); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	s.notifyChatEventGetMembers("chat.update.members.add", chatid, nil)
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

	if err := s.repo.DeleteMembers(m.Chatid, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove chat members")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, time.Now().Round(0).UnixMilli()); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat")
	}
	s.notifyChatEventGetMembers("chat.update.members.remove", chatid, userids)
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
	members, err := s.repo.GetMembers(chatid, chatMemberAmountCap*2, 0)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get chat members")
	}
	if err := s.repo.DeleteChatMembers(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete chat members")
	}
	userids := make([]string, 0, len(members))
	for _, i := range members {
		userids = append(userids, i.Userid)
	}
	s.notifyChatEvent("chat.delete", chatid, userids)
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

type (
	resChats struct {
		Chats []resChat `json:"chats"`
	}
)

func (s *service) GetChats(chatids []string) (*resChats, error) {
	m, err := s.repo.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chats")
	}
	vchatids := make([]string, 0, len(m))
	for _, i := range m {
		vchatids = append(vchatids, i.Chatid)
	}
	allMembers, err := s.repo.GetChatsMembers(vchatids, len(vchatids)*chatMemberAmountCap)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get chat members")
	}
	membersByChat := map[string][]string{}
	for _, i := range vchatids {
		membersByChat[i] = []string{}
	}
	for _, i := range allMembers {
		membersByChat[i.Chatid] = append(membersByChat[i.Chatid], i.Userid)
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
			Members:      membersByChat[i.Chatid],
		})
	}
	return &resChats{
		Chats: chats,
	}, nil
}

func (s *service) GetLatestChatsByKind(kind string, userid string, before int64, limit int) (*resChats, error) {
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

type (
	resMsg struct {
		Chatid string `json:"chatid"`
		Msgid  string `json:"msgid"`
		Userid string `json:"userid"`
		Timems int64  `json:"time_ms"`
		Kind   string `json:"kind"`
		Value  string `json:"value"`
	}
)

func (s *service) CreateMsg(chatid string, userid string, kind string, value string) (*resMsg, error) {
	m, err := s.repo.GetChat(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Chat not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get chat")
	}

	msg, err := s.repo.AddMsg(m.Chatid, userid, kind, value)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create chat msg")
	}
	if err := s.repo.InsertMsg(msg); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to insert chat msg")
	}
	if err := s.repo.UpdateChatLastUpdated(m.Chatid, msg.Timems); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update chat")
	}
	s.notifyChatEventGetMembers("chat.msg.send", chatid, nil)

	return &resMsg{
		Chatid: msg.Chatid,
		Msgid:  msg.Msgid,
		Userid: msg.Userid,
		Timems: msg.Timems,
		Kind:   msg.Kind,
		Value:  msg.Value,
	}, nil
}

type (
	resMsgs struct {
		Msgs []resMsg `json:"msgs"`
	}
)

func (s *service) GetLatestChatMsgsByKind(chatid string, kind string, before string, limit int) (*resMsgs, error) {
	var msgs []model.MsgModel
	if kind == "" {
		if before == "" {
			var err error
			msgs, err = s.repo.GetMsgs(chatid, limit, 0)
			if err != nil {
				return nil, governor.ErrWithMsg(err, "Failed to get latest msgs")
			}
		} else {
			var err error
			msgs, err = s.repo.GetMsgsBefore(chatid, before, limit)
			if err != nil {
				return nil, governor.ErrWithMsg(err, "Failed to get latest msgs")
			}
		}
	} else {
		if before == "" {
			var err error
			msgs, err = s.repo.GetMsgsByKind(chatid, kind, limit, 0)
			if err != nil {
				return nil, governor.ErrWithMsg(err, "Failed to get latest msgs")
			}
		} else {
			var err error
			msgs, err = s.repo.GetMsgsBeforeByKind(chatid, kind, before, limit)
			if err != nil {
				return nil, governor.ErrWithMsg(err, "Failed to get latest msgs")
			}
		}
	}

	res := make([]resMsg, 0, len(msgs))
	for _, i := range msgs {
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
