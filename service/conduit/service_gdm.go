package conduit

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/gdm/model"
	"xorkevin.dev/governor/service/db"
)

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
	groupChatMemberCap = 127
)

type (
	resGDM struct {
		Chatid       string   `json:"chatid"`
		Name         string   `json:"name"`
		Theme        string   `json:"theme"`
		LastUpdated  int64    `json:"last_updated"`
		CreationTime int64    `json:"creation_time"`
		Members      []string `json:"members"`
	}

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
