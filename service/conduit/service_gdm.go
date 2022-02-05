package conduit

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/gdm/model"
	"xorkevin.dev/governor/service/db"
)

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

func uniqStrs(a []string) []string {
	res := make([]string, 0, len(a))
	set := map[string]struct{}{}
	for _, i := range a {
		if _, ok := set[i]; ok {
			continue
		}
		res = append(res, i)
		set[i] = struct{}{}
	}
	return res
}

type (
	resGDM struct {
		Chatid       string   `json:"chatid"`
		Name         string   `json:"name"`
		Theme        string   `json:"theme"`
		LastUpdated  int64    `json:"last_updated"`
		CreationTime int64    `json:"creation_time"`
		Members      []string `json:"members"`
	}
)

func (s *service) CreateGDM(name string, theme string, requserids []string) (*resGDM, error) {
	userids := uniqStrs(requserids)
	if len(userids) != len(requserids) {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Must provide unique users",
		}))
	}
	if len(userids) < 3 {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "May not create group chat with less than 3 users",
		}))
	}

	if err := s.checkUsersExist(userids); err != nil {
		return nil, err
	}

	m, err := s.gdms.New(name, theme)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new group chat")
	}
	if err := s.gdms.Insert(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new group chat")
	}
	if _, err := s.gdms.AddMembers(m.Chatid, userids); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to add members to new group chat")
	}
	return &resGDM{
		Chatid:       m.Chatid,
		Name:         m.Name,
		Theme:        m.Theme,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
		Members:      userids,
	}, nil
}

func (s *service) getGDMByChatid(userid string, chatid string) (*model.Model, error) {
	members, err := s.gdms.GetMembers(chatid, []string{userid})
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chat members")
	}
	if len(members) != 1 {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusNotFound,
			Message: "Group chat not found",
		}), governor.ErrOptInner(err))
	}
	m, err := s.gdms.GetByID(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Group chat not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get group chat")
	}
	return m, nil
}

func (s *service) UpdateGDM(userid string, chatid string, name, theme string) error {
	m, err := s.getGDMByChatid(userid, chatid)
	if err != nil {
		return err
	}
	m.Name = name
	m.Theme = theme
	if err := s.gdms.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update group chat")
	}
	// TODO publish gdm settings event
	return nil
}

const (
	groupChatMemberCap = 31
)

func (s *service) AddGDMMembers(userid string, chatid string, reqmembers []string) error {
	if len(reqmembers) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "No users to add",
		}))
	}

	members := uniqStrs(reqmembers)
	if len(members) != len(reqmembers) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Must provide unique users",
		}))
	}

	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return err
	}

	if err := s.checkUsersExist(members); err != nil {
		return err
	}

	count, err := s.gdms.GetMembersCount(chatid)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get group chat members count")
	}
	if count+len(members) > groupChatMemberCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "May not have more than 31 group chat members",
		}), governor.ErrOptInner(err))
	}

	now, err := s.gdms.AddMembers(chatid, members)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to add group chat members")
	}

	if err := s.gdms.UpdateLastUpdated(chatid, now); err != nil {
		return governor.ErrWithMsg(err, "Failed to update group chat last updated")
	}

	// TODO publish member added event
	return nil
}

func (s *service) RmGDMMembers(userid string, chatid string, reqmembers []string) error {
	if len(reqmembers) == 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "No users to remove",
		}))
	}

	members := uniqStrs(reqmembers)
	if len(members) != len(reqmembers) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Must provide unique users",
		}))
	}

	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return err
	}

	found, err := s.gdms.GetMembers(chatid, members)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get group chat members")
	}
	if len(found) != len(members) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Member does not exist",
		}))
	}

	count, err := s.gdms.GetMembersCount(chatid)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get group chat members count")
	}
	if count-len(found) < 3 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Group chat must have at least 3 users",
		}), governor.ErrOptInner(err))
	}

	if err := s.gdms.RmMembers(chatid, members); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove group chat members")
	}

	if err := s.gdms.UpdateLastUpdated(chatid, time.Now().Round(0).UnixMilli()); err != nil {
		return governor.ErrWithMsg(err, "Failed to update group chat last updated")
	}

	// TODO publish member removed event
	return nil
}

func (s *service) DeleteGDM(userid string, chatid string) error {
	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return err
	}

	if err := s.msgs.DeleteChatMsgs(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete group chat messages")
	}
	if err := s.gdms.Delete(chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete group chat")
	}

	// TODO publish chat delete event
	return nil
}

type (
	resGDMs struct {
		GDMs []resGDM `json:"gdms"`
	}
)

func (s *service) getGDMsWithMembers(chatids []string) (*resGDMs, error) {
	m, err := s.gdms.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chats")
	}
	members, err := s.gdms.GetChatsMembers(chatids, len(chatids)*groupChatMemberCap*2)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chat members")
	}
	memMap := map[string][]string{}
	for _, i := range members {
		memMap[i.Chatid] = append(memMap[i.Chatid], i.Userid)
	}
	res := make([]resGDM, 0, len(m))
	for _, i := range m {
		res = append(res, resGDM{
			Chatid:       i.Chatid,
			Name:         i.Name,
			Theme:        i.Theme,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
			Members:      memMap[i.Chatid],
		})
	}
	return &resGDMs{
		GDMs: res,
	}, nil
}

func (s *service) GetLatestGDMs(userid string, before int64, limit int) (*resGDMs, error) {
	chatids, err := s.gdms.GetLatest(userid, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest group chats")
	}
	return s.getGDMsWithMembers(chatids)
}

func (s *service) GetGDMs(userid string, reqchatids []string) (*resGDMs, error) {
	chatids, err := s.gdms.GetUserChats(userid, reqchatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chats")
	}
	return s.getGDMsWithMembers(chatids)
}

func (s *service) SearchGDMs(userid1, userid2 string, limit, offset int) (*resGDMs, error) {
	chatids, err := s.gdms.GetAssocs(userid1, userid2, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to search group chats")
	}
	return s.getGDMsWithMembers(chatids)
}

func (s *service) CreateGDMMsg(userid string, chatid string, kind string, value string) (*resMsg, error) {
	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return nil, err
	}
	m, err := s.msgs.New(chatid, userid, kind, value)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new group chat msg")
	}
	if err := s.msgs.Insert(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to send new group chat msg")
	}
	if err := s.gdms.UpdateLastUpdated(chatid, m.Timems); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update group chat last updated")
	}
	res := resMsg{
		Chatid: m.Chatid,
		Msgid:  m.Msgid,
		Userid: m.Userid,
		Timems: m.Timems,
		Kind:   m.Kind,
		Value:  m.Value,
	}
	// TODO publish gdm message send event
	return &res, nil
}

func (s *service) GetGDMMsgs(userid string, chatid string, kind string, before string, limit int) (*resMsgs, error) {
	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return nil, err
	}
	m, err := s.msgs.GetMsgs(chatid, kind, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chat msgs")
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

func (s *service) DelGDMMsg(userid string, chatid string, msgid string) error {
	if _, err := s.getGDMByChatid(userid, chatid); err != nil {
		return err
	}
	if err := s.msgs.DeleteMsgs(chatid, []string{msgid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete group chat msgs")
	}
	// TODO: emit msg delete event
	return nil
}