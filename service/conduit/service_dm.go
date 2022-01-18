package conduit

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

func (s *service) UpdateDM(userid1, userid2 string, name, theme string) error {
	m, err := s.dms.GetByID(userid1, userid2)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "DM not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get dm")
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
		DMs []resDMSearch
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
