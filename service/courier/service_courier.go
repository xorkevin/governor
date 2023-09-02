package courier

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/couriermodel"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	cacheValTombstone = "-"
)

type (
	resGetLink struct {
		LinkID       string `json:"linkid"`
		URL          string `json:"url"`
		CreatorID    string `json:"creatorid"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (s *Service) getLink(ctx context.Context, linkid string) (*resGetLink, error) {
	m, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get link")
	}
	return &resGetLink{
		LinkID:       m.LinkID,
		URL:          m.URL,
		CreatorID:    m.CreatorID,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *Service) getLinkFast(ctx context.Context, linkid string) (string, error) {
	if cachedURL, err := s.kvlinks.Get(ctx, linkid); err != nil {
		if !errors.Is(err, kvstore.ErrNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to get linkid url from cache"))
		}
	} else if cachedURL == cacheValTombstone {
		return "", governor.ErrWithRes(nil, http.StatusNotFound, "", "Link not found")
	} else {
		return cachedURL, nil
	}
	res, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			if err := s.kvlinks.Set(ctx, linkid, cacheValTombstone, s.cacheDuration); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to cache linkid url"))
			}
			return "", governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return "", kerrors.WithMsg(err, "Failed to get link")
	}
	if err := s.kvlinks.Set(ctx, linkid, res.URL, s.cacheDuration); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to cache linkid url"),
			klog.AString("linkid", linkid),
		)
	}
	return res.URL, nil
}

type (
	resLinkGroup struct {
		Links []resGetLink `json:"links"`
	}
)

func (s *Service) getLinkGroup(ctx context.Context, creatorid string, limit, offset int) (*resLinkGroup, error) {
	links, err := s.repo.GetLinkGroup(ctx, creatorid, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get links")
	}
	res := make([]resGetLink, 0, len(links))
	for _, i := range links {
		res = append(res, resGetLink{
			LinkID:       i.LinkID,
			URL:          i.URL,
			CreatorID:    i.CreatorID,
			CreationTime: i.CreationTime,
		})
	}
	return &resLinkGroup{
		Links: res,
	}, nil
}

const (
	qrScale = 8
)

type (
	resCreateLink struct {
		LinkID string `json:"linkid"`
	}
)

func (s *Service) createLink(ctx context.Context, creatorid, linkid, url string) (*resCreateLink, error) {
	var m *couriermodel.LinkModel
	if len(linkid) == 0 {
		var err error
		m, err = s.repo.NewLinkAuto(creatorid, url)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate link id")
		}
	} else {
		m = s.repo.NewLink(creatorid, linkid, url)
	}

	if err := s.repo.InsertLink(ctx, m); err != nil {
		if errors.Is(err, dbsql.ErrUnique) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "Link id already taken")
		}
		return nil, kerrors.WithMsg(err, "Failed to create link")
	}

	return &resCreateLink{
		LinkID: m.LinkID,
	}, nil
}

func (s *Service) deleteLink(ctx context.Context, creatorid, linkid string) error {
	m, err := s.repo.GetLink(ctx, linkid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Link not found")
		}
		return kerrors.WithMsg(err, "Failed to get link")
	}
	if m.CreatorID != creatorid {
		return governor.ErrWithRes(nil, http.StatusNotFound, "", "Link not found")
	}
	if err := s.repo.DeleteLink(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete link")
	}
	// must give a best effort attempt to clear the cache
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.kvlinks.Del(ctx, linkid); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to delete linkid url"),
			klog.AString("linkid", linkid),
		)
	}
	return nil
}
