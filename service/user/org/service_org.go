package org

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
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

func (s *service) GetByID(orgid string) (*resOrg, error) {
	m, err := s.orgs.GetByID(orgid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Org not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get org")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Org not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get org")
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
		return nil, governor.ErrWithMsg(err, "Failed to get orgs")
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
		return nil, governor.ErrWithMsg(err, "Failed to get orgs")
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

func (s *service) CreateOrg(userid, displayName, desc string) (*resOrg, error) {
	m, err := s.orgs.New(displayName, desc)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to create org")
	}
	if err := s.orgs.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Org name must be unique",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to insert org")
	}
	orgrole := rank.ToOrgName(m.OrgID)
	if err := s.roles.InsertRoles(userid, rank.Rank{}.AddMod(orgrole).AddUsr(orgrole)); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to add mod roles to user")
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
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Org not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get org")
	}
	m.Name = name
	m.DisplayName = displayName
	m.Desc = desc
	if err := s.orgs.Update(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Org name must be unique",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to update org")
	}
	return nil
}

func (s *service) DeleteOrg(orgid string) error {
	m, err := s.orgs.GetByID(orgid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Org not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get org")
	}
	orgrole := rank.ToOrgName(orgid)
	if err := s.roles.DeleteByRole(rank.ToUsrName(orgrole)); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove org users")
	}
	if err := s.roles.DeleteByRole(rank.ToModName(orgrole)); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove org mods")
	}
	if err := s.orgs.Delete(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete org")
	}
	return nil
}
