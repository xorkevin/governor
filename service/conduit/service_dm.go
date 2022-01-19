package conduit

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/dm/model"
	"xorkevin.dev/governor/service/db"
)

const (
	chatMsgKindTxt = "t"
)

func (s *service) getDMByChatid(userid string, chatid string) (*model.Model, error) {
	m, err := s.dms.GetByChatID(chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "DM not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get dm")
	}
	if m.Userid1 != userid && m.Userid2 != userid {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusNotFound,
			Message: "DM not found",
		}))
	}
	return m, nil
}

func (s *service) UpdateDM(userid string, chatid string, name, theme string) error {
	m, err := s.getDMByChatid(userid, chatid)
	if err != nil {
		return err
	}
	m.Name = name
	m.Theme = theme
	if err := s.dms.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update dm")
	}
	// TODO: notify dm event
	return nil
}

type (
	resDM struct {
		Userid       string `json:"userid"`
		Chatid       string `json:"chatid"`
		Name         string `json:"name"`
		Theme        string `json:"theme"`
		LastUpdated  int64  `json:"last_updated"`
		CreationTime int64  `json:"creation_time"`
	}

	resDMs struct {
		DMs []resDM `json:"dms"`
	}
)

func useridDiff(a, b, c string) string {
	if a == b {
		return c
	}
	return b
}

func (s *service) GetLatestDMs(userid string, before int64, limit int) (*resDMs, error) {
	chatids, err := s.dms.GetLatest(userid, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest dms")
	}
	m, err := s.dms.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get dms")
	}
	res := make([]resDM, 0, len(m))
	for _, i := range m {
		res = append(res, resDM{
			Userid:       useridDiff(userid, i.Userid1, i.Userid2),
			Chatid:       i.Chatid,
			Name:         i.Name,
			Theme:        i.Theme,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
		})
	}
	return &resDMs{
		DMs: res,
	}, nil
}

func (s *service) GetDMs(userid string, chatids []string) (*resDMs, error) {
	m, err := s.dms.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get dms")
	}
	res := make([]resDM, 0, len(m))
	for _, i := range m {
		if i.Userid1 != userid && i.Userid2 != userid {
			continue
		}
		res = append(res, resDM{
			Userid:       useridDiff(userid, i.Userid1, i.Userid2),
			Chatid:       i.Chatid,
			Name:         i.Name,
			Theme:        i.Theme,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
		})
	}
	return &resDMs{
		DMs: res,
	}, nil
}

type (
	resDMSearch struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
		Chatid   string `json:"chatid"`
		Name     string `json:"name"`
	}

	resDMSearches struct {
		DMs []resDMSearch `json:"dms"`
	}
)

func (s *service) SearchDMs(userid string, prefix string, limit int) (*resDMSearches, error) {
	m, err := s.friends.GetFriends(userid, prefix, limit, 0)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to search friends")
	}
	usernames := map[string]string{}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid2)
		usernames[i.Userid2] = i.Username
	}
	chatInfo, err := s.dms.GetByUser(userid, userids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get dms")
	}
	res := make([]resDMSearch, 0, len(chatInfo))
	for _, i := range chatInfo {
		k := useridDiff(userid, i.Userid1, i.Userid2)
		res = append(res, resDMSearch{
			Userid:   k,
			Username: usernames[k],
			Chatid:   i.Chatid,
			Name:     i.Name,
		})
	}
	return &resDMSearches{
		DMs: res,
	}, nil
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

func (s *service) CreateDMMsg(userid string, chatid string, kind string, value string) (*resMsg, error) {
	if _, err := s.getDMByChatid(userid, chatid); err != nil {
		return nil, err
	}
	m, err := s.msgs.New(chatid, userid, kind, value)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create new dm msg")
	}
	if err := s.msgs.Insert(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to send new dm msg")
	}
	// TODO: notify dm event
	return &resMsg{
		Chatid: m.Chatid,
		Msgid:  m.Msgid,
		Userid: m.Userid,
		Timems: m.Timems,
		Kind:   m.Kind,
		Value:  m.Value,
	}, nil
}

type (
	resMsgs struct {
		Msgs []resMsg `json:"msgs"`
	}
)

func (s *service) GetDMMsgs(userid string, chatid string, kind string, before string, limit int) (*resMsgs, error) {
	if _, err := s.getDMByChatid(userid, chatid); err != nil {
		return nil, err
	}
	m, err := s.msgs.GetMsgs(chatid, kind, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get dm msgs")
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

func (s *service) DelDMMsg(userid string, chatid string, msgid string) error {
	if _, err := s.getDMByChatid(userid, chatid); err != nil {
		return err
	}
	if err := s.msgs.DeleteMsgs(chatid, []string{msgid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete dm msgs")
	}
	return nil
}
