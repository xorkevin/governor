package conduit

import (
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
)

type (
	resServer struct {
		ServerID     string `json:"serverid"`
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		Theme        string `json:"theme"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *service) CreateServer(serverid string, name, desc string, theme string) (*resServer, error) {
	m := s.servers.New(serverid, name, desc, theme)
	if err := s.servers.Insert(m); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusConflict,
				Message: "Server already created",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to create server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) GetServer(serverid string) (*resServer, error) {
	m, err := s.servers.GetServer(serverid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get server")
	}
	return &resServer{
		ServerID:     m.ServerID,
		Name:         m.Name,
		Desc:         m.Desc,
		Theme:        m.Theme,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) UpdateServer(serverid string, name, desc string, theme string) error {
	m, err := s.servers.GetServer(serverid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Server not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get server")
	}
	m.Name = name
	m.Desc = desc
	m.Theme = theme
	if err := s.servers.Update(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update server")
	}
	return nil
}
