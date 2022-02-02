package conduit

import (
	"xorkevin.dev/governor"
)

type (
	resGDM struct {
		Chatid       string `json:"chatid"`
		Name         string `json:"name"`
		Theme        string `json:"theme"`
		LastUpdated  int64  `json:"last_updated"`
		CreationTime int64  `json:"creation_time"`
	}

	resGDMs struct {
		GDMs []resGDM `json:"gdms"`
	}
)

func (s *service) GetLatestGDMs(userid string, before int64, limit int) (*resGDMs, error) {
	chatids, err := s.gdms.GetLatest(userid, before, limit)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest group chats")
	}
	m, err := s.gdms.GetChats(chatids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get group chats")
	}
	res := make([]resGDM, 0, len(m))
	for _, i := range m {
		res = append(res, resGDM{
			Chatid:       i.Chatid,
			Name:         i.Name,
			Theme:        i.Theme,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
		})
	}
	return &resGDMs{
		GDMs: res,
	}, nil
}
