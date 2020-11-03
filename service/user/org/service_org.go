package org

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/rank"
)

type (
	resOrg struct {
		OrgID        string `json:"orgid"`
		Name         string `json:"name"`
		DisplayName  string `json:"display_name"`
		Desc         string `json:"desc"`
		CreationTime int64  `json:"creation_time"`
	}

	resOrgs struct {
		Orgs []resOrg `json:"orgs"`
	}
)

func orgRole(orgid string) string {
	return "org_" + orgid
}

func (s *service) GetByID(orgid string) (*resOrg, error) {
	m, err := s.orgs.GetByID(orgid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetByName(name string) (*resOrg, error) {
	m, err := s.orgs.GetByName(name)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetOrgs(orgids []string) (*resOrgs, error) {
	m, err := s.orgs.GetOrgs(orgids)
	if err != nil {
		return nil, err
	}
	orgs := make([]resOrg, 0, len(m))
	for _, i := range m {
		orgs = append(orgs, resOrg{
			OrgID:        i.OrgID,
			Name:         i.Name,
			DisplayName:  i.DisplayName,
			Desc:         i.Desc,
			CreationTime: i.CreationTime,
		})
	}
	return &resOrgs{
		Orgs: orgs,
	}, nil
}

func (s *service) GetAllOrgs(limit, offset int) (*resOrgs, error) {
	m, err := s.orgs.GetAllOrgs(limit, offset)
	if err != nil {
		return nil, err
	}
	orgs := make([]resOrg, 0, len(m))
	for _, i := range m {
		orgs = append(orgs, resOrg{
			OrgID:        i.OrgID,
			Name:         i.Name,
			DisplayName:  i.DisplayName,
			Desc:         i.Desc,
			CreationTime: i.CreationTime,
		})
	}
	return &resOrgs{
		Orgs: orgs,
	}, nil
}

func (s *service) CreateOrg(userid, name, displayName, desc string) (*resOrg, error) {
	if _, err := s.orgs.GetByName(name); err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return nil, err
		}
	} else {
		return nil, governor.NewErrorUser("Org name already taken", http.StatusBadRequest, nil)
	}

	m, err := s.orgs.New(name, displayName, desc)
	if err != nil {
		return nil, err
	}
	if err := s.orgs.Insert(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	orgrole := orgRole(m.OrgID)
	if err := s.roles.InsertRoles(userid, rank.Rank{}.AddMod(orgrole).AddUser(orgrole)); err != nil {
		return nil, governor.NewError("Failed to add mod roles to user", 0, err)
	}
	return &resOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) UpdateOrg(orgid, name, displayName, desc string) error {
	m, err := s.orgs.GetByID(orgid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	m.Name = name
	m.DisplayName = displayName
	m.Desc = desc
	if err := s.orgs.Update(m); err != nil {
		if governor.ErrorStatus(err) == http.StatusBadRequest {
			return governor.NewErrorUser("Org name must be unique", 0, err)
		}
		return err
	}
	return nil
}

func (s *service) DeleteOrg(orgid string) error {
	m, err := s.orgs.GetByID(orgid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	orgrole := orgRole(orgid)
	if err := s.roles.DeleteByRole(rank.Rank{}.AddUser(orgrole).Stringify()); err != nil {
		return governor.NewError("Failed to remove org users", 0, err)
	}
	if err := s.roles.DeleteByRole(rank.Rank{}.AddMod(orgrole).Stringify()); err != nil {
		return governor.NewError("Failed to remove org mods", 0, err)
	}
	if err := s.orgs.Delete(m); err != nil {
		return err
	}
	return nil
}
