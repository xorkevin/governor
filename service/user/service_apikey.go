package user

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

type (
	resApikey struct {
		Keyid    string `json:"keyid"`
		AuthTags string `json:"auth_tags"`
		Name     string `json:"name"`
		Desc     string `json:"desc"`
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
			Name:     i.Name,
			Desc:     i.Desc,
			Time:     i.Time,
		})
	}
	return &resApikeys{
		Apikeys: res,
	}, nil
}

type (
	resApikeyModel struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}
)

func (s *service) CreateApikey(userid string, authtags rank.Rank, name, desc string) (*resApikeyModel, error) {
	intersect, err := s.roles.IntersectRoles(userid, authtags)
	if err != nil {
		return nil, err
	}
	m, key, err := s.apikeys.New(userid, intersect, name, desc)
	if err != nil {
		return nil, err
	}
	if err := s.apikeys.Insert(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *service) RotateApikey(keyid string) (*resApikeyModel, error) {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	key, err := s.apikeys.RehashKey(m)
	if err != nil {
		return nil, err
	}
	if err := s.apikeys.Update(m); err != nil {
		return nil, err
	}
	return &resApikeyModel{
		Keyid: m.Keyid,
		Key:   key,
	}, nil
}

func (s *service) UpdateApikey(userid, keyid string, authtags rank.Rank, name, desc string) error {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	intersect, err := s.roles.IntersectRoles(userid, authtags)
	if err != nil {
		return err
	}
	m.AuthTags = intersect
	m.Name = name
	m.Desc = desc
	if err := s.apikeys.Update(m); err != nil {
		return err
	}
	return nil
}

func (s *service) DeleteApikey(keyid string) error {
	m, err := s.apikeys.GetByID(keyid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if err := s.apikeys.Delete(m); err != nil {
		return err
	}
	return nil
}

func (s *service) DeleteUserApikeys(userid string) error {
	if err := s.apikeys.DeleteUserKeys(userid); err != nil {
		return err
	}
	return nil
}
