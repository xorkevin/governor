package mailinglist

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
)

type (
	resList struct {
		ListID       string `json:"listid"`
		CreatorID    string `json:"creatorid"`
		Listname     string `json:"listname"`
		Name         string `json:"name"`
		Description  string `json:"desc"`
		Archive      bool   `json:"archive"`
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
		Listname:     list.Listname,
		Name:         list.Name,
		Description:  list.Description,
		Archive:      list.Archive,
		SenderPolicy: list.SenderPolicy,
		MemberPolicy: list.MemberPolicy,
		LastUpdated:  list.LastUpdated,
		CreationTime: list.CreationTime,
	}, nil
}

func (s *service) UpdateList(creatorid string, listname string, name, desc string, archive bool, senderPolicy, memberPolicy string) error {
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
	m.Archive = archive
	m.SenderPolicy = senderPolicy
	m.MemberPolicy = memberPolicy
	if err := s.lists.UpdateList(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list")
	}
	return nil
}

func (s *service) checkUsersExist(userids []string) error {
	ids, err := s.users.CheckUsersExist(userids)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to users exist check")
	}
	if len(ids) != len(userids) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "User does not exist",
		}))
	}
	return nil
}

func (s *service) Subscribe(creatorid string, listname string, userid string) error {
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
	isOrg := rank.IsValidOrgName(creatorid)
	switch m.MemberPolicy {
	case listMemberPolicyOwner:
		if isOrg {
			if ok, err := gate.AuthMember(s.gate, userid, creatorid); err != nil {
				return governor.ErrWithMsg(err, "Failed to get user membership")
			} else if !ok {
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusForbidden,
					Message: "Not a member of the org",
				}))
			}
		} else {
			if userid != creatorid {
				return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusForbidden,
					Message: "Not the list owner",
				}))
			}
		}
	case listMemberPolicyUser:
	default:
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusConflict,
			Message: "Invalid list member policy",
		}))
	}
	if members, err := s.lists.GetListMembers(m.ListID, []string{userid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to get list members")
	} else if len(members) != 0 {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "List member already added",
		}), governor.ErrOptInner(err))
	}
	if count, err := s.lists.GetMembersCount(m.ListID); err != nil {
		return governor.ErrWithMsg(err, "Failed to get list members count")
	} else if count+1 > mailingListMemberAmountCap {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "May not have more than 255 list members",
		}), governor.ErrOptInner(err))
	}

	if err := s.checkUsersExist([]string{userid}); err != nil {
		return err
	}

	members := s.lists.AddMembers(m, []string{userid})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to update list")
	}
	if err := s.lists.InsertMembers(members); err != nil {
		return governor.ErrWithMsg(err, "Failed to add list members")
	}
	return nil
}

func (s *service) RemoveListMembers(creatorid string, listname string, userids []string) error {
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
	if members, err := s.lists.GetListMembers(m.ListID, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to get list members")
	} else if len(members) != len(userids) {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusNotFound,
			Message: "List member does not exist",
		}), governor.ErrOptInner(err))
	}
	if err := s.lists.DeleteMembers(m.ListID, userids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove list members")
	}
	return nil
}

func (s *service) DeleteList(creatorid string, listname string) error {
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
	j, err := json.Marshal(delmsg{
		ListID: m.ListID,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode list delete message")
	}
	if err := s.events.StreamPublish(s.opts.DelChannel, j); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish list delete event")
	}
	if err := s.lists.DeleteListTrees(m.ListID); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list trees")
	}
	if err := s.lists.DeleteListMembers(m.ListID); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list members")
	}
	if err := s.lists.DeleteList(m); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete list")
	}
	return nil
}

func (s *service) GetList(listid string) (*resList, error) {
	m, err := s.lists.GetListByID(listid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	return &resList{
		ListID:       m.ListID,
		CreatorID:    m.CreatorID,
		Listname:     m.Listname,
		Name:         m.Name,
		Description:  m.Description,
		Archive:      m.Archive,
		SenderPolicy: m.SenderPolicy,
		MemberPolicy: m.MemberPolicy,
		LastUpdated:  m.LastUpdated,
		CreationTime: m.CreationTime,
	}, nil
}

type (
	resListMemberIDs struct {
		Members []string `json:"members"`
	}
)

func (s *service) GetListMembers(listid string, amount, offset int) (*resListMemberIDs, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	members, err := s.lists.GetMembers(listid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get list members")
	}
	ids := make([]string, 0, len(members))
	for _, i := range members {
		ids = append(ids, i.Userid)
	}
	return &resListMemberIDs{
		Members: ids,
	}, nil
}

func (s *service) GetListMemberIDs(listid string, userids []string) (*resListMemberIDs, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	members, err := s.lists.GetListMembers(listid, userids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get list members")
	}
	ids := make([]string, 0, len(members))
	for _, i := range members {
		ids = append(ids, i.Userid)
	}
	return &resListMemberIDs{
		Members: ids,
	}, nil
}

type (
	resLists struct {
		Lists []resList `json:"lists"`
	}
)

func (s *service) GetLists(listids []string) (*resLists, error) {
	m, err := s.lists.GetLists(listids)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get lists")
	}
	vlistids := make([]string, 0, len(m))
	for _, i := range m {
		vlistids = append(vlistids, i.ListID)
	}
	lists := make([]resList, 0, len(m))
	for _, i := range m {
		lists = append(lists, resList{
			ListID:       i.ListID,
			CreatorID:    i.CreatorID,
			Listname:     i.Listname,
			Name:         i.Name,
			Description:  i.Description,
			Archive:      i.Archive,
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

func (s *service) GetCreatorLists(creatorid string, amount, offset int) (*resLists, error) {
	m, err := s.lists.GetCreatorLists(creatorid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest lists")
	}
	listids := make([]string, 0, len(m))
	for _, i := range m {
		listids = append(listids, i.ListID)
	}
	lists := make([]resList, 0, len(m))
	for _, i := range m {
		lists = append(lists, resList{
			ListID:       i.ListID,
			CreatorID:    i.CreatorID,
			Listname:     i.Listname,
			Name:         i.Name,
			Description:  i.Description,
			Archive:      i.Archive,
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

func (s *service) GetLatestLists(userid string, amount, offset int) (*resLists, error) {
	m, err := s.lists.GetLatestLists(userid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get latest lists")
	}
	listids := make([]string, 0, len(m))
	for _, i := range m {
		listids = append(listids, i.ListID)
	}
	return s.GetLists(listids)
}

func (s *service) DeleteMsgs(creatorid string, listname string, msgids []string) error {
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
	for _, i := range msgids {
		if err := s.rcvMailDir.Subdir(m.ListID).Del(base64.RawURLEncoding.EncodeToString([]byte(i))); err != nil {
			if !errors.Is(err, objstore.ErrNotFound{}) {
				return governor.ErrWithMsg(err, "Failed to delete msg content")
			}
		}
	}
	if err := s.lists.DeleteSentMsgLogs(m.ListID, msgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete sent message logs")
	}
	if err := s.lists.DeleteMsgs(m.ListID, msgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete messages")
	}
	return nil
}

type (
	resMsg struct {
		ListID       string `json:"listid"`
		Msgid        string `json:"msgid"`
		Userid       string `json:"userid"`
		CreationTime int64  `json:"creation_time"`
		SPFPass      string `json:"spf_pass"`
		DKIMPass     string `json:"dkim_pass"`
		Subject      string `json:"subject"`
		InReplyTo    string `json:"in_reply_to"`
		ParentID     string `json:"parent_id"`
		ThreadID     string `json:"thread_id"`
		Deleted      bool   `json:"deleted"`
	}

	resMsgs struct {
		Msgs []resMsg `json:"msgs"`
	}
)

func (s *service) GetMsg(listid, msgid string) (*resMsg, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(listid, msgid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Message not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get message")
	}
	return &resMsg{
		ListID:       m.ListID,
		Msgid:        m.Msgid,
		Userid:       m.Userid,
		CreationTime: m.CreationTime,
		SPFPass:      m.SPFPass,
		DKIMPass:     m.DKIMPass,
		Subject:      m.Subject,
		InReplyTo:    m.InReplyTo,
		ParentID:     m.ParentID,
		ThreadID:     m.ThreadID,
		Deleted:      m.Deleted,
	}, nil
}

func (s *service) GetLatestMsgs(listid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListMsgs(listid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get messages")
	}
	msgs := make([]resMsg, 0, len(m))
	for _, i := range m {
		msgs = append(msgs, resMsg{
			ListID:       i.ListID,
			Msgid:        i.Msgid,
			Userid:       i.Userid,
			CreationTime: i.CreationTime,
			SPFPass:      i.SPFPass,
			DKIMPass:     i.DKIMPass,
			Subject:      i.Subject,
			InReplyTo:    i.InReplyTo,
			ParentID:     i.ParentID,
			ThreadID:     i.ThreadID,
			Deleted:      i.Deleted,
		})
	}
	return &resMsgs{
		Msgs: msgs,
	}, nil
}

func (s *service) GetLatestThreads(listid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListThreads(listid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get threads")
	}
	msgs := make([]resMsg, 0, len(m))
	for _, i := range m {
		msgs = append(msgs, resMsg{
			ListID:       i.ListID,
			Msgid:        i.Msgid,
			Userid:       i.Userid,
			CreationTime: i.CreationTime,
			SPFPass:      i.SPFPass,
			DKIMPass:     i.DKIMPass,
			Subject:      i.Subject,
			InReplyTo:    i.InReplyTo,
			ParentID:     i.ParentID,
			ThreadID:     i.ThreadID,
			Deleted:      i.Deleted,
		})
	}
	return &resMsgs{
		Msgs: msgs,
	}, nil
}

func (s *service) GetThreadMsgs(listid, threadid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(listid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListThread(listid, threadid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get thread")
	}
	msgs := make([]resMsg, 0, len(m))
	for _, i := range m {
		msgs = append(msgs, resMsg{
			ListID:       i.ListID,
			Msgid:        i.Msgid,
			Userid:       i.Userid,
			CreationTime: i.CreationTime,
			SPFPass:      i.SPFPass,
			DKIMPass:     i.DKIMPass,
			Subject:      i.Subject,
			InReplyTo:    i.InReplyTo,
			ParentID:     i.ParentID,
			ThreadID:     i.ThreadID,
			Deleted:      i.Deleted,
		})
	}
	return &resMsgs{
		Msgs: msgs,
	}, nil
}

func (s *service) StatMsg(listid, msgid string) (*objstore.ObjectInfo, error) {
	m, err := s.lists.GetListByID(listid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get list")
	}
	objinfo, err := s.rcvMailDir.Subdir(m.ListID).Stat(base64.RawURLEncoding.EncodeToString([]byte(msgid)))
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Msg content not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get msg content")
	}
	return objinfo, nil
}

func (s *service) GetMsgContent(listid, msgid string) (io.ReadCloser, string, error) {
	m, err := s.lists.GetListByID(listid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "List not found",
			}), governor.ErrOptInner(err))
		}
		return nil, "", governor.ErrWithMsg(err, "Failed to get list")
	}
	obj, objinfo, err := s.rcvMailDir.Subdir(m.ListID).Get(base64.RawURLEncoding.EncodeToString([]byte(msgid)))
	if err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			return nil, "", governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Msg content not found",
			}), governor.ErrOptInner(err))
		}
		return nil, "", governor.ErrWithMsg(err, "Failed to get msg content")
	}
	return obj, objinfo.ContentType, nil
}

type (
	delmsg struct {
		ListID string `json:"listid"`
	}
)

const (
	msgDeleteBatchSize = 256
)

func (s *service) deleteSubscriber(pinger events.Pinger, msgdata []byte) error {
	msg := &delmsg{}
	if err := json.Unmarshal(msgdata, msg); err != nil {
		return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decode list delete message")
	}

	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		msgs, err := s.lists.GetListMsgs(msg.ListID, msgDeleteBatchSize, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get list messages")
		}
		if len(msgs) == 0 {
			break
		}
		msgids := make([]string, 0, len(msgs))
		for _, i := range msgs {
			if err := s.rcvMailDir.Subdir(i.ListID).Del(base64.RawURLEncoding.EncodeToString([]byte(i.Msgid))); err != nil {
				if !errors.Is(err, objstore.ErrNotFound{}) {
					return governor.ErrWithMsg(err, "Failed to delete msg content")
				}
			}
			msgids = append(msgids, i.Msgid)
		}
		if err := s.lists.DeleteSentMsgLogs(msg.ListID, msgids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete sent message logs")
		}
		if err := s.lists.DeleteMsgs(msg.ListID, msgids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete list messages")
		}
		if len(msgs) < msgDeleteBatchSize {
			break
		}
	}
	return nil
}

type (
	mailmsg struct {
		ListID string `json:"listid"`
		MsgID  string `json:"msgid"`
		From   string `json:"from"`
	}

	sendmsg struct {
		ListID string `json:"listid"`
		MsgID  string `json:"msgid"`
		From   string `json:"from"`
	}

	// ErrMailEvent is returned when the msgqueue mail message is malformed
	ErrMailEvent struct{}
)

func (e ErrMailEvent) Error() string {
	return "Malformed mail message"
}

func (s *service) mailSubscriber(pinger events.Pinger, msgdata []byte) error {
	emmsg := &mailmsg{}
	if err := json.Unmarshal(msgdata, emmsg); err != nil {
		return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decode mail message")
	}

	ml, err := s.lists.GetListByID(emmsg.ListID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("List not found", map[string]string{
				"actiontype": "getmaillist",
				"error":      err.Error(),
			})
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(emmsg.ListID, emmsg.MsgID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("Msg not found", map[string]string{
				"actiontype": "getmailmsg",
				"error":      err.Error(),
			})
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get list msg")
	}
	if !m.Processed {
		if err := s.lists.MarkMsgProcessed(m.ListID, m.Msgid); err != nil {
			return governor.ErrWithMsg(err, "Failed to mark list msg")
		}
	}
	if err := s.lists.InsertTree(s.lists.NewTree(m.ListID, m.Msgid, m.CreationTime)); err != nil {
		if !errors.Is(err, db.ErrUnique{}) {
			return governor.ErrWithMsg(err, "Failed to insert list thread tree")
		}
	}
	threadid := m.Msgid
	if m.InReplyTo != "" {
		if p, err := s.lists.GetMsg(m.ListID, m.InReplyTo); err != nil {
			if !errors.Is(err, db.ErrNotFound{}) {
				return governor.ErrWithMsg(err, "Failed to get list msg parent")
			}
			// parent not found
		} else {
			// parent exists
			if err := s.lists.InsertTreeEdge(m.ListID, m.Msgid, p.Msgid); err != nil {
				return governor.ErrWithMsg(err, "Failed to insert list thread edge")
			}

			threadid = p.Msgid
			if k := p.ThreadID; k != "" {
				threadid = k
			}
			if m.ParentID != "" {
				if err := s.lists.UpdateMsgParent(m.ListID, m.Msgid, p.Msgid, threadid); err != nil {
					return governor.ErrWithMsg(err, "Failed to update list msg parent")
				}
			}
		}
	}
	// thread updates must occur before children updates since thread updates are
	// culled by non thread children
	if err := s.lists.InsertTreeChildren(m.ListID, m.Msgid); err != nil {
		return governor.ErrWithMsg(err, "Failed to insert list thread children")
	}
	if err := s.lists.UpdateMsgThread(m.ListID, m.Msgid, threadid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list msg thread")
	}
	if err := s.lists.UpdateMsgChildren(m.ListID, m.Msgid, threadid); err != nil {
		return governor.ErrWithMsg(err, "Failed to update list msg children")
	}
	if m.CreationTime > ml.CreationTime {
		if err := s.lists.UpdateListLastUpdated(m.ListID, m.CreationTime); err != nil {
			return governor.ErrWithMsg(err, "Failed to update list last updated")
		}
	}

	j, err := json.Marshal(sendmsg{
		ListID: emmsg.ListID,
		MsgID:  emmsg.MsgID,
		From:   emmsg.From,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode mail send message")
	}
	if err := s.events.StreamPublish(s.opts.SendChannel, j); err != nil {
		return governor.ErrWithMsg(err, "Failed to publish mail send event")
	}
	return nil
}

const (
	mailingListSendBatchSize = 256
)

func (s *service) sendSubscriber(pinger events.Pinger, msgdata []byte) error {
	emmsg := &sendmsg{}
	if err := json.Unmarshal(msgdata, emmsg); err != nil {
		return governor.ErrWithKind(err, ErrMailEvent{}, "Failed to decode mail message")
	}

	if _, err := s.lists.GetListByID(emmsg.ListID); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("List not found", map[string]string{
				"actiontype": "getmaillist",
				"error":      err.Error(),
			})
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(emmsg.ListID, emmsg.MsgID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			s.logger.Error("Msg not found", map[string]string{
				"actiontype": "getmailmsg",
				"error":      err.Error(),
			})
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get list msg")
	}
	if m.Sent {
		if err := s.lists.DeleteSentMsgLogs(m.ListID, []string{m.Msgid}); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete sent message logs")
		}
		return nil
	}

	mb := bytes.Buffer{}
	if err := func() error {
		obj, _, err := s.rcvMailDir.Subdir(m.ListID).Get(base64.RawURLEncoding.EncodeToString([]byte(m.Msgid)))
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get msg content")
		}
		defer func() {
			if err := obj.Close(); err != nil {
				s.logger.Error("Failed to close msg content", map[string]string{
					"actiontype": "getlistmsg",
					"error":      err.Error(),
				})
			}
		}()
		if _, err := io.Copy(&mb, obj); err != nil {
			return governor.ErrWithMsg(err, "Failed to read msg content")
		}
		return nil
	}(); err != nil {
		if errors.Is(err, objstore.ErrNotFound{}) {
			s.logger.Error("Msg content not found", map[string]string{
				"actiontype": "getlistmsgcontent",
				"error":      err.Error(),
			})
			if err := s.lists.MarkMsgSent(m.ListID, m.Msgid); err != nil {
				return governor.ErrWithMsg(err, "Failed to mark list message sent")
			}
			if err := s.lists.DeleteSentMsgLogs(m.ListID, []string{m.Msgid}); err != nil {
				return governor.ErrWithMsg(err, "Failed to delete sent message logs")
			}
			return nil
		}
		return err
	}

	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		userids, err := s.lists.GetUnsentMsgs(emmsg.ListID, emmsg.MsgID, mailingListSendBatchSize)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get unsent messages")
		}
		if len(userids) == 0 {
			break
		}
		recipients, err := s.users.GetInfoBulk(userids)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get list member users")
		}
		if len(recipients.Users) > 0 {
			rcpts := make([]string, 0, len(recipients.Users))
			for _, i := range recipients.Users {
				rcpts = append(rcpts, i.Email)
			}
			if err := s.mailer.FwdStream(emmsg.From, rcpts, int64(mb.Len()), bytes.NewReader(mb.Bytes()), false); err != nil {
				return governor.ErrWithMsg(err, "Failed to send mail message")
			}
		}
		if err := s.lists.LogSentMsg(emmsg.ListID, emmsg.MsgID, userids); err != nil {
			return governor.ErrWithMsg(err, "Failed to log sent mail messages")
		}
		if len(userids) < mailingListSendBatchSize {
			break
		}
	}

	if err := s.lists.MarkMsgSent(m.ListID, m.Msgid); err != nil {
		return governor.ErrWithMsg(err, "Failed to mark list message sent")
	}
	if err := s.lists.DeleteSentMsgLogs(m.ListID, []string{m.Msgid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete sent message logs")
	}
	return nil
}
