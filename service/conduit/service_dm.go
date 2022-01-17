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
