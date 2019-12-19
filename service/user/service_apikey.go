package user

import (
	"net/http"
	"xorkevin.dev/governor"
)

type (
	resApikey struct {
		Keyid    string `json:"keyid"`
		AuthTags string `json:"authtags"`
		Time     int64  `json:"time"`
	}

	resApikeys struct {
		Apikeys []resApikey `json:"apikeys"`
	}
)

func (s *service) GetUserApikeys(userid string, limit, offset int) (*resApikeys, error) {
	m, err := s.apikeys.GetUserKeys(userid, limit, offset)
	if err != nil {
		return nil, err
	}
	res := make([]resApikey, 0, len(m))
	for _, i := range m {
		res = append(res, resApikey{
			Keyid:    i.Keyid,
			AuthTags: i.AuthTags.Stringify(),
			Time:     i.Time,
		})
	}
	return &resApikeys{
		Apikeys: res,
	}, nil
}
