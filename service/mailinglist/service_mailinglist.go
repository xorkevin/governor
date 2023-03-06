package mailinglist

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
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

func (s *Service) createList(ctx context.Context, creatorid string, listname string, name, desc string, senderPolicy, memberPolicy string) (*resList, error) {
	list := s.lists.NewList(creatorid, listname, name, desc, senderPolicy, memberPolicy)
	if err := s.lists.InsertList(ctx, list); err != nil {
		if errors.Is(err, db.ErrorUnique) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "List id already taken")
		}
		return nil, kerrors.WithMsg(err, "Failed to create list")
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

func (s *Service) updateList(ctx context.Context, creatorid string, listname string, name, desc string, archive bool, senderPolicy, memberPolicy string) error {
	m, err := s.lists.GetList(ctx, creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	m.Name = name
	m.Description = desc
	m.Archive = archive
	m.SenderPolicy = senderPolicy
	m.MemberPolicy = memberPolicy
	if err := s.lists.UpdateList(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to update list")
	}
	return nil
}

func (s *Service) checkUsersExist(ctx context.Context, userids []string) error {
	ids, err := s.users.CheckUsersExist(ctx, userids)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to users exist check")
	}
	if len(ids) != len(userids) {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "User does not exist")
	}
	return nil
}

func (s *Service) subscribe(ctx context.Context, creatorid string, listname string, userid string) error {
	m, err := s.lists.GetList(ctx, creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	isOrg := rank.IsValidOrgName(creatorid)
	switch m.MemberPolicy {
	case listMemberPolicyOwner:
		if isOrg {
			if ok, err := gate.AuthMember(ctx, s.gate, userid, creatorid); err != nil {
				return kerrors.WithMsg(err, "Failed to get user membership")
			} else if !ok {
				return governor.ErrWithRes(nil, http.StatusForbidden, "", "Not the list owner")
			}
		} else {
			if userid != creatorid {
				return governor.ErrWithRes(nil, http.StatusForbidden, "", "Not the list owner")
			}
		}
	case listMemberPolicyUser:
	default:
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid list member policy")
	}
	if members, err := s.lists.GetListMembers(ctx, m.ListID, []string{userid}); err != nil {
		return kerrors.WithMsg(err, "Failed to get list members")
	} else if len(members) != 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "List member already added")
	}

	if err := s.checkUsersExist(ctx, []string{userid}); err != nil {
		return err
	}

	members := s.lists.AddMembers(m, []string{userid})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to update list")
	}
	if err := s.lists.InsertMembers(ctx, members); err != nil {
		return kerrors.WithMsg(err, "Failed to add list members")
	}
	return nil
}

func (s *Service) removeListMembers(ctx context.Context, creatorid string, listname string, userids []string) error {
	m, err := s.lists.GetList(ctx, creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	if members, err := s.lists.GetListMembers(ctx, m.ListID, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to get list members")
	} else if len(members) != len(userids) {
		return governor.ErrWithRes(err, http.StatusNotFound, "", "List member does not exist")
	}
	if err := s.lists.DeleteMembers(ctx, m.ListID, userids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove list members")
	}
	return nil
}

func (s *Service) deleteList(ctx context.Context, creatorid string, listname string) error {
	m, err := s.lists.GetList(ctx, creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	b, err := encodeListEventDelete(delProps{
		ListID: m.ListID,
	})
	if err != nil {
		return err
	}
	if err := s.events.Publish(ctx, events.NewMsgs(s.streammail, m.ListID, b)...); err != nil {
		return kerrors.WithMsg(err, "Failed to publish list delete event")
	}
	if err := s.lists.DeleteList(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list")
	}
	return nil
}

func (s *Service) getList(ctx context.Context, listid string) (*resList, error) {
	m, err := s.lists.GetListByID(ctx, listid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
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

func (s *Service) getListMembers(ctx context.Context, listid string, amount, offset int) (*resListMemberIDs, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	members, err := s.lists.GetMembers(ctx, listid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list members")
	}
	ids := make([]string, 0, len(members))
	for _, i := range members {
		ids = append(ids, i.Userid)
	}
	return &resListMemberIDs{
		Members: ids,
	}, nil
}

func (s *Service) getListMemberIDs(ctx context.Context, listid string, userids []string) (*resListMemberIDs, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	members, err := s.lists.GetListMembers(ctx, listid, userids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get list members")
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

func (s *Service) getLists(ctx context.Context, listids []string) (*resLists, error) {
	m, err := s.lists.GetLists(ctx, listids)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get lists")
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

func (s *Service) getCreatorLists(ctx context.Context, creatorid string, amount, offset int) (*resLists, error) {
	m, err := s.lists.GetCreatorLists(ctx, creatorid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest lists")
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

func (s *Service) getLatestLists(ctx context.Context, userid string, amount, offset int) (*resLists, error) {
	m, err := s.lists.GetLatestLists(ctx, userid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get latest lists")
	}
	listids := make([]string, 0, len(m))
	for _, i := range m {
		listids = append(listids, i.ListID)
	}
	return s.getLists(ctx, listids)
}

func (s *Service) encodeMsgid(msgid string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(msgid))
}

func (s *Service) deleteMsgs(ctx context.Context, creatorid string, listname string, msgids []string) error {
	m, err := s.lists.GetList(ctx, creatorid, listname)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	for _, i := range msgids {
		if err := s.rcvMailDir.Subdir(m.ListID).Del(ctx, s.encodeMsgid(i)); err != nil {
			if !errors.Is(err, objstore.ErrorNotFound) {
				return kerrors.WithMsg(err, "Failed to delete msg content")
			}
		}
	}
	if err := s.lists.DeleteSentMsgLogs(ctx, m.ListID, msgids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sent message logs")
	}
	if err := s.lists.DeleteMsgs(ctx, m.ListID, msgids); err != nil {
		return kerrors.WithMsg(err, "Failed to delete messages")
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

func (s *Service) getMsg(ctx context.Context, listid, msgid string) (*resMsg, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(ctx, listid, msgid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Message not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get message")
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

func (s *Service) getLatestMsgs(ctx context.Context, listid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListMsgs(ctx, listid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get messages")
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

func (s *Service) getLatestThreads(ctx context.Context, listid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListThreads(ctx, listid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get threads")
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

func (s *Service) getThreadMsgs(ctx context.Context, listid, threadid string, amount, offset int) (*resMsgs, error) {
	if _, err := s.lists.GetListByID(ctx, listid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetListThread(ctx, listid, threadid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get thread")
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

func (s *Service) statMsg(ctx context.Context, listid, msgid string) (*objstore.ObjectInfo, error) {
	m, err := s.lists.GetListByID(ctx, listid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get list")
	}
	objinfo, err := s.rcvMailDir.Subdir(m.ListID).Stat(ctx, s.encodeMsgid(msgid))
	if err != nil {
		if errors.Is(err, objstore.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "Msg content not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get msg content")
	}
	return objinfo, nil
}

func (s *Service) getMsgContent(ctx context.Context, listid, msgid string) (io.ReadCloser, string, error) {
	m, err := s.lists.GetListByID(ctx, listid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "List not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get list")
	}
	obj, objinfo, err := s.rcvMailDir.Subdir(m.ListID).Get(ctx, s.encodeMsgid(msgid))
	if err != nil {
		if errors.Is(err, objstore.ErrorNotFound) {
			return nil, "", governor.ErrWithRes(err, http.StatusNotFound, "", "Msg content not found")
		}
		return nil, "", kerrors.WithMsg(err, "Failed to get msg content")
	}
	return obj, objinfo.ContentType, nil
}

const (
	msgDeleteBatchSize = 256
)

func (s *Service) deleteEventHandler(ctx context.Context, props delProps) error {
	if err := s.lists.DeleteListMembers(ctx, props.ListID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list members")
	}
	if err := s.lists.DeleteListTrees(ctx, props.ListID); err != nil {
		return kerrors.WithMsg(err, "Failed to delete list trees")
	}

	for {
		msgs, err := s.lists.GetListMsgs(ctx, props.ListID, msgDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get list messages")
		}
		if len(msgs) == 0 {
			break
		}
		msgids := make([]string, 0, len(msgs))
		for _, i := range msgs {
			if err := s.rcvMailDir.Subdir(i.ListID).Del(ctx, s.encodeMsgid(i.Msgid)); err != nil {
				if !errors.Is(err, objstore.ErrorNotFound) {
					return kerrors.WithMsg(err, "Failed to delete msg content")
				}
			}
			msgids = append(msgids, i.Msgid)
		}
		if err := s.lists.DeleteSentMsgLogs(ctx, props.ListID, msgids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete sent message logs")
		}
		if err := s.lists.DeleteMsgs(ctx, props.ListID, msgids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete list messages")
		}
		if len(msgs) < msgDeleteBatchSize {
			break
		}
	}
	return nil
}

func (s *Service) mailEventHandler(ctx context.Context, props mailProps) error {
	b, err := encodeListEventSend(sendProps{
		ListID: props.ListID,
		MsgID:  props.MsgID,
	})
	if err != nil {
		return err
	}

	ml, err := s.lists.GetListByID(ctx, props.ListID)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "List not found"))
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(ctx, props.ListID, props.MsgID)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Msg not found"))
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get list msg")
	}
	if !m.Processed {
		if err := s.lists.MarkMsgProcessed(ctx, m.ListID, m.Msgid); err != nil {
			return kerrors.WithMsg(err, "Failed to mark list msg")
		}
	}
	// In a closure table, every node must also point to itself with depth 0, so
	// insert a node that does that.
	if err := s.lists.InsertTree(ctx, s.lists.NewTree(m.ListID, m.Msgid, m.CreationTime)); err != nil {
		if !errors.Is(err, db.ErrorUnique) {
			return kerrors.WithMsg(err, "Failed to insert list thread tree")
		}
	}
	threadid := m.Msgid
	if m.InReplyTo != "" {
		if p, err := s.lists.GetMsg(ctx, m.ListID, m.InReplyTo); err != nil {
			if !errors.Is(err, db.ErrorNotFound) {
				return kerrors.WithMsg(err, "Failed to get list msg parent")
			}
			// parent not found
		} else {
			// parent exists

			// A message's parent may not be updated, so all messages must be in the
			// form of a tree, and will not form a more general DAG.

			// Add parent closures for the message
			if err := s.lists.InsertTreeEdge(ctx, m.ListID, m.Msgid, p.Msgid); err != nil {
				return kerrors.WithMsg(err, "Failed to insert list thread edge")
			}

			threadid = p.Msgid
			if p.ThreadID != "" {
				threadid = p.ThreadID
			}
			if err := s.lists.UpdateMsgParent(ctx, m.ListID, m.Msgid, p.Msgid, threadid); err != nil {
				return kerrors.WithMsg(err, "Failed to update list msg parent")
			}
		}
	}
	// Update any children closures for the message if they exist. This depends
	// on the messages table not having been updated for any message with the
	// current message as its parent. Thus this must occur before updating
	// message parents.
	if err := s.lists.InsertTreeChildren(ctx, m.ListID, m.Msgid); err != nil {
		return kerrors.WithMsg(err, "Failed to insert list thread children")
	}
	// Like updating children closures, this depends on the messages table not
	// having been updated for any message with the current message as its
	// parent. Thus this must occur before updating message parents.
	if err := s.lists.UpdateMsgThread(ctx, m.ListID, m.Msgid, threadid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list msg thread")
	}
	// Finally, update the message's direct children's parents and threads
	if err := s.lists.UpdateMsgChildren(ctx, m.ListID, m.Msgid, threadid); err != nil {
		return kerrors.WithMsg(err, "Failed to update list msg children")
	}
	if m.CreationTime > ml.CreationTime {
		if err := s.lists.UpdateListLastUpdated(ctx, m.ListID, m.CreationTime); err != nil {
			return kerrors.WithMsg(err, "Failed to update list last updated")
		}
	}

	if err := s.events.Publish(ctx, events.NewMsgs(s.streammail, props.ListID, b)...); err != nil {
		return kerrors.WithMsg(err, "Failed to publish mail send event")
	}
	return nil
}

const (
	mailingListSendBatchSize = 256
)

func (s *Service) sendEventHandler(ctx context.Context, props sendProps) error {
	if _, err := s.lists.GetListByID(ctx, props.ListID); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "List not found"))
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get list")
	}
	m, err := s.lists.GetMsg(ctx, props.ListID, props.MsgID)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Msg not found"))
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get list msg")
	}
	if m.Sent || m.Deleted {
		if err := s.lists.DeleteSentMsgLogs(ctx, m.ListID, []string{m.Msgid}); err != nil {
			return kerrors.WithMsg(err, "Failed to delete sent message logs")
		}
		return nil
	}

	mb := &bytes.Buffer{}
	if err := func() error {
		obj, _, err := s.rcvMailDir.Subdir(m.ListID).Get(ctx, s.encodeMsgid(m.Msgid))
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get msg content")
		}
		defer func() {
			if err := obj.Close(); err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close msg content"))
			}
		}()
		if _, err := io.Copy(mb, obj); err != nil {
			return kerrors.WithMsg(err, "Failed to read msg content")
		}
		return nil
	}(); err != nil {
		if errors.Is(err, objstore.ErrorNotFound) {
			s.log.Err(ctx, kerrors.WithMsg(err, "Msg content not found"))
			if err := s.lists.MarkMsgSent(ctx, m.ListID, m.Msgid); err != nil {
				return kerrors.WithMsg(err, "Failed to mark list message sent")
			}
			if err := s.lists.DeleteSentMsgLogs(ctx, m.ListID, []string{m.Msgid}); err != nil {
				return kerrors.WithMsg(err, "Failed to delete sent message logs")
			}
			return nil
		}
		return err
	}

	for {
		userids, err := s.lists.GetUnsentMsgs(ctx, props.ListID, props.MsgID, mailingListSendBatchSize)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get unsent messages")
		}
		if len(userids) == 0 {
			break
		}
		recipients, err := s.users.GetInfoBulk(ctx, userids)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get list member users")
		}
		if len(recipients.Users) > 0 {
			rcpts := make([]string, 0, len(recipients.Users))
			for _, i := range recipients.Users {
				rcpts = append(rcpts, i.Email)
			}
			if err := s.mailer.FwdStream(ctx, "", rcpts, int64(mb.Len()), bytes.NewReader(mb.Bytes()), false); err != nil {
				return kerrors.WithMsg(err, "Failed to send mail message")
			}
		}
		if err := s.lists.LogSentMsg(ctx, props.ListID, props.MsgID, userids); err != nil {
			return kerrors.WithMsg(err, "Failed to log sent mail messages")
		}
		if len(userids) < mailingListSendBatchSize {
			break
		}
	}

	if err := s.lists.MarkMsgSent(ctx, m.ListID, m.Msgid); err != nil {
		return kerrors.WithMsg(err, "Failed to mark list message sent")
	}
	if err := s.lists.DeleteSentMsgLogs(ctx, m.ListID, []string{m.Msgid}); err != nil {
		return kerrors.WithMsg(err, "Failed to delete sent message logs")
	}
	return nil
}
