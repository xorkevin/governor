package org

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
)

type (
	ErrorNotFound struct{}
)

func (e ErrorNotFound) Error() string {
	return "Org not found"
}

type (
	ResOrg struct {
		OrgID        string `json:"orgid"`
		Name         string `json:"name"`
		DisplayName  string `json:"display_name"`
		Desc         string `json:"desc"`
		CreationTime int64  `json:"creation_time"`
	}

	resOrgs struct {
		Orgs []ResOrg `json:"orgs"`
	}

	resMember struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
	}

	resMembers struct {
		Members []resMember `json:"members"`
	}
)

func (s *service) GetByID(ctx context.Context, orgid string) (*ResOrg, error) {
	m, err := s.orgs.GetByID(ctx, orgid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrorNotFound{}, "Org not found"), http.StatusNotFound, "", "Org not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get org")
	}
	return &ResOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetByName(ctx context.Context, name string) (*ResOrg, error) {
	m, err := s.orgs.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return nil, governor.ErrWithRes(kerrors.WithKind(err, ErrorNotFound{}, "Org not found"), http.StatusNotFound, "", "Org not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get org")
	}
	return &ResOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) getOrgs(ctx context.Context, orgids []string) (*resOrgs, error) {
	m, err := s.orgs.GetOrgs(ctx, orgids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get orgs")
	}
	orgs := make([]ResOrg, 0, len(m))
	for _, i := range m {
		orgs = append(orgs, ResOrg{
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

func (s *service) getAllOrgs(ctx context.Context, limit, offset int) (*resOrgs, error) {
	m, err := s.orgs.GetAllOrgs(ctx, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get orgs")
	}
	orgs := make([]ResOrg, 0, len(m))
	for _, i := range m {
		orgs = append(orgs, ResOrg{
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

func (s *service) getOrgMembers(ctx context.Context, orgid string, prefix string, limit, offset int) (*resMembers, error) {
	m, err := s.orgs.GetOrgMembers(ctx, orgid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org members")
	}
	res := make([]resMember, 0, len(m))
	for _, i := range m {
		res = append(res, resMember{
			Userid:   i.Userid,
			Username: i.Username,
		})
	}
	return &resMembers{
		Members: res,
	}, nil
}

func (s *service) getOrgMods(ctx context.Context, orgid string, prefix string, limit, offset int) (*resMembers, error) {
	m, err := s.orgs.GetOrgMods(ctx, orgid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get org mods")
	}
	res := make([]resMember, 0, len(m))
	for _, i := range m {
		res = append(res, resMember{
			Userid:   i.Userid,
			Username: i.Username,
		})
	}
	return &resMembers{
		Members: res,
	}, nil
}

func (s *service) getOrgsByID(ctx context.Context, orgids []string) (*resOrgs, error) {
	m, err := s.orgs.GetOrgs(ctx, orgids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user orgs")
	}
	orgMap := map[string]model.Model{}
	for _, i := range m {
		orgMap[i.OrgID] = i
	}
	res := make([]ResOrg, 0, len(orgMap))
	for _, i := range orgids {
		k, ok := orgMap[i]
		if !ok {
			continue
		}
		res = append(res, ResOrg{
			OrgID:        k.OrgID,
			Name:         k.Name,
			DisplayName:  k.DisplayName,
			Desc:         k.Desc,
			CreationTime: k.CreationTime,
		})
	}
	return &resOrgs{
		Orgs: res,
	}, nil
}

func (s *service) getUserOrgs(ctx context.Context, userid string, prefix string, limit, offset int) (*resOrgs, error) {
	orgids, err := s.orgs.GetUserOrgs(ctx, userid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user orgs")
	}
	return s.getOrgsByID(ctx, orgids)
}

func (s *service) getUserMods(ctx context.Context, userid string, prefix string, limit, offset int) (*resOrgs, error) {
	orgids, err := s.orgs.GetUserMods(ctx, userid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get user mod orgs")
	}
	return s.getOrgsByID(ctx, orgids)
}

func (s *service) createOrg(ctx context.Context, userid, displayName, desc string) (*ResOrg, error) {
	m, err := s.orgs.New(displayName, desc)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create org")
	}
	if err := s.orgs.Insert(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique{}) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "Org name must be unique")
		}
		return nil, kerrors.WithMsg(err, "Failed to insert org")
	}
	orgrole := rank.ToOrgName(m.OrgID)
	if err := s.roles.InsertRoles(ctx, userid, rank.Rank{}.AddMod(orgrole).AddUsr(orgrole)); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to add mod roles to user")
	}
	return &ResOrg{
		OrgID:        m.OrgID,
		Name:         m.Name,
		DisplayName:  m.DisplayName,
		Desc:         m.Desc,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) updateOrg(ctx context.Context, orgid, name, displayName, desc string) error {
	m, err := s.orgs.GetByID(ctx, orgid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Org not found")
		}
		return kerrors.WithMsg(err, "Failed to get org")
	}
	updName := m.Name != name
	m.Name = name
	m.DisplayName = displayName
	m.Desc = desc
	if err := s.orgs.Update(ctx, m); err != nil {
		if errors.Is(err, db.ErrorUnique{}) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Org name must be unique")
		}
		return kerrors.WithMsg(err, "Failed to update org")
	}
	if updName {
		if err := s.orgs.UpdateName(ctx, m.OrgID, m.Name); err != nil {
			return kerrors.WithMsg(err, "Failed to update org name")
		}
	}
	return nil
}

func (s *service) deleteOrg(ctx context.Context, orgid string) error {
	m, err := s.orgs.GetByID(ctx, orgid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Org not found")
		}
		return kerrors.WithMsg(err, "Failed to get org")
	}
	b, err := kjson.Marshal(DeleteOrgProps{
		OrgID: m.OrgID,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode org props to json")
	}
	if err := s.events.StreamPublish(ctx, s.opts.DeleteChannel, b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish delete org event")
	}
	if err := s.orgs.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete org")
	}
	return nil
}
