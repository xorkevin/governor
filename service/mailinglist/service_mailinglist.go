package mailinglist

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/mailinglist/model"
)

type (
	resList struct {
		ListID       string `json:"listid"`
		CreatorID    string `json:"creatorid"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		SenderPolicy string `json:"sender_policy"`
		MemberPolicy string `json:"member_policy"`
		LastUpdated  int64  `json:"last_updated"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *service) CreateList(creatorid string, listname string, name, desc string, senderPolicy, memberPolicy string) (*resList, error) {
	list := s.lists.NewList(creatorid, listname, name, desc, senderPolicy, memberPolicy)
	if err := s.lists.InsertList(list); err != nil {
		if errors.Is(err, db.ErrUnique{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "List id already taken",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to create list")
	}
	return &resList{
		ListID:       list.ListID,
		CreatorID:    list.CreatorID,
		Name:         list.Name,
		Description:  list.Description,
		SenderPolicy: list.SenderPolicy,
		MemberPolicy: list.MemberPolicy,
		LastUpdated:  list.LastUpdated,
		CreationTime: list.CreationTime,
	}, nil
}

func (s *service) UpdateList(creatorid string, listname string, name, desc string, senderPolicy, memberPolicy string) error {
	m, err := s.lists.GetList(creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get list")
	}
	m.Name = name
	m.Description = desc
	m.SenderPolicy = senderPolicy
	m.MemberPolicy = memberPolicy
	m.LastUpdated = time.Now().Round(0).UnixMilli()
	if err := s.lists.UpdateList(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list")
	}
	if err := s.lists.UpdateListLastUpdated(m.ListID, m.LastUpdated); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list")
	}
	return nil
}

type (
	resLists struct {
		Lists []resList `json:"lists"`
	}
)

func (s *service) GetLatestLists(creatorid string, before int64, limit int) (*resLists, error) {
	var m []model.ListModel
	if before == 0 {
		var err error
		m, err = s.lists.GetCreatorLists(creatorid, limit, 0)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to get latest lists")
		}
	} else {
		var err error
		m, err = s.lists.GetCreatorListsBefore(creatorid, before, limit)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to get latest lists")
		}
	}
	lists := make([]resList, 0, len(m))
	for _, i := range m {
		lists = append(lists, resList{
			ListID:       i.ListID,
			CreatorID:    i.CreatorID,
			Name:         i.Name,
			Description:  i.Description,
			SenderPolicy: i.SenderPolicy,
			MemberPolicy: i.MemberPolicy,
			LastUpdated:  i.LastUpdated,
			CreationTime: i.CreationTime,
		})
	}
	return &resLists{
		Lists: lists,
	}, nil
}
