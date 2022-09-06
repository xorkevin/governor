package conduit

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/dm/model"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/kerrors"
)

func (s *service) publishDMMsgEvent(userids []string, v interface{}) {
	if len(userids) == 0 {
		return
	}

	present, err := s.getPresence(context.Background(), s.logger, locDM, userids)
	if err != nil {
		s.logger.Error("Failed to get presence", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_get_dm_presence",
		})
		return
	}
	// must make a best effort attempt to publish dm msg event
	for _, i := range present {
		if err := s.ws.Publish(context.Background(), i, s.opts.DMMsgChannel, v); err != nil {
			s.logger.Error("Failed to publish dm msg event", map[string]string{
				"error":      err.Error(),
				"actiontype": "conduit_publish_dm_msg",
			})
		}
	}
}

func (s *service) publishDMSettingsEvent(userids []string, v interface{}) {
	if len(userids) == 0 {
		return
	}

	present, err := s.getPresence(context.Background(), s.logger, locDM, userids)
	if err != nil {
		s.logger.Error("Failed to get presence", map[string]string{
			"error":      err.Error(),
			"actiontype": "conduit_get_dm_presence",
		})
		return
	}
	for _, i := range present {
		if err := s.ws.Publish(context.Background(), i, s.opts.DMSettingsChannel, v); err != nil {
			s.logger.Error("Failed to publish dm settings event", map[string]string{
				"error":      err.Error(),
				"actiontype": "conduit_publish_dm_settings",
			})
		}
	}
}

const (
	chatMsgKindTxt = "t"
)

func (s *service) getDMByChatid(ctx context.Context, userid string, chatid string) (*model.Model, error) {
	m, err := s.dms.GetByChatID(ctx, chatid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "DM not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get dm")
	}
	if m.Userid1 != userid && m.Userid2 != userid {
		return nil, governor.ErrWithRes(nil, http.StatusNotFound, "", "DM not found")
	}
	return m, nil
}

type (
	resDMID struct {
		Chatid string `json:"chatid"`
	}
)

func (s *service) UpdateDM(ctx context.Context, userid string, chatid string, name, theme string) error {
	m, err := s.getDMByChatid(ctx, userid, chatid)
	if err != nil {
		return err
	}
	m.Name = name
	m.Theme = theme
	if err := s.dms.UpdateProps(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update dm")
	}
	s.publishDMSettingsEvent([]string{m.Userid1, m.Userid2}, resDMID{
		Chatid: m.Chatid,
	})
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

func (s *service) GetLatestDMs(ctx context.Context, userid string, before int64, limit int) (*resDMs, error) {
	m, err := s.dms.GetLatest(ctx, userid, before, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest dms")
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

func (s *service) GetDMs(ctx context.Context, userid string, chatids []string) (*resDMs, error) {
	m, err := s.dms.GetChats(ctx, chatids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dms")
	}
	res := make([]resDM, 0, len(m))
	for _, i := range m {
		if i.Userid1 != userid && i.Userid2 != userid {
			continue
		}
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

type (
	resDMSearch struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
		Chatid   string `json:"chatid"`
		Name     string `json:"name"`
	}

	resDMSearches struct {
		DMs []resDMSearch `json:"dms"`
	}
)

func (s *service) SearchDMs(ctx context.Context, userid string, prefix string, limit int) (*resDMSearches, error) {
	m, err := s.friends.GetFriends(ctx, userid, prefix, limit, 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to search friends")
	}
	usernames := map[string]string{}
	userids := make([]string, 0, len(m))
	for _, i := range m {
		userids = append(userids, i.Userid2)
		usernames[i.Userid2] = i.Username
	}
	chatInfo, err := s.dms.GetByUser(ctx, userid, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dms")
	}
	res := make([]resDMSearch, 0, len(chatInfo))
	for _, i := range chatInfo {
		k := useridDiff(userid, i.Userid1, i.Userid2)
		res = append(res, resDMSearch{
			Userid:   k,
			Username: usernames[k],
			Chatid:   i.Chatid,
			Name:     i.Name,
		})
	}
	return &resDMSearches{
		DMs: res,
	}, nil
}

type (
	resMsg struct {
		Chatid string `json:"chatid"`
		Msgid  string `json:"msgid"`
		Userid string `json:"userid"`
		Timems int64  `json:"time_ms"`
		Kind   string `json:"kind"`
		Value  string `json:"value"`
	}
)

func (s *service) CreateDMMsg(ctx context.Context, userid string, chatid string, kind string, value string) (*resMsg, error) {
	dm, err := s.getDMByChatid(ctx, userid, chatid)
	if err != nil {
		return nil, err
	}
	m, err := s.msgs.New(chatid, userid, kind, value)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to create new dm msg")
	}
	if err := s.msgs.Insert(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to send new dm msg")
	}
	if err := s.dms.UpdateLastUpdated(ctx, dm.Userid1, dm.Userid2, m.Timems); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to update dm last updated")
	}
	res := resMsg{
		Chatid: m.Chatid,
		Msgid:  m.Msgid,
		Userid: m.Userid,
		Timems: m.Timems,
		Kind:   m.Kind,
		Value:  m.Value,
	}
	s.publishDMMsgEvent([]string{dm.Userid1, dm.Userid2}, res)
	return &res, nil
}

type (
	resMsgs struct {
		Msgs []resMsg `json:"msgs"`
	}
)

func (s *service) GetDMMsgs(ctx context.Context, userid string, chatid string, kind string, before string, limit int) (*resMsgs, error) {
	if _, err := s.getDMByChatid(ctx, userid, chatid); err != nil {
		return nil, err
	}
	m, err := s.msgs.GetMsgs(ctx, chatid, kind, before, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get dm msgs")
	}
	res := make([]resMsg, 0, len(m))
	for _, i := range m {
		res = append(res, resMsg{
			Chatid: i.Chatid,
			Msgid:  i.Msgid,
			Userid: i.Userid,
			Timems: i.Timems,
			Kind:   i.Kind,
			Value:  i.Value,
		})
	}
	return &resMsgs{
		Msgs: res,
	}, nil
}

func (s *service) DelDMMsg(ctx context.Context, userid string, chatid string, msgid string) error {
	if _, err := s.getDMByChatid(ctx, userid, chatid); err != nil {
		return err
	}
	if err := s.msgs.EraseMsgs(ctx, chatid, []string{msgid}); err != nil {
		return kerrors.WithMsg(err, "Failed to delete dm msg")
	}
	// TODO: emit msg delete event
	return nil
}
