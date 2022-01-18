package conduit

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
)

type (
	resFriends struct {
		Friends []string `json:"friends"`
	}
)

func (s *service) GetFriends(userid string, prefix string, limit, offset int) (*resFriends, error) {
	m, err := s.friends.GetFriends(userid, prefix, limit, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to search friends")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Userid2)
	}
	return &resFriends{
		Friends: res,
	}, nil
}

func (s *service) RemoveFriend(userid1, userid2 string) error {
	if _, err := s.friends.GetByID(userid1, userid2); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "Not friends",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get friends")
	}
	b, err := json.Marshal(UnfriendProps{
		Userid: userid1,
		Other:  userid2,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode unfriend props to json")
	}
	if err := s.friends.Remove(userid1, userid2); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove friend")
	}
	if err := s.events.StreamPublish(s.opts.UnfriendChannel, b); err != nil {
		s.logger.Error("Failed to publish unfriend event", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishunfriend",
		})
	}
	return nil
}

func (s *service) InviteFriend(userid string, invitedBy string) error {
	if _, err := s.friends.GetByID(userid, invitedBy); err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return governor.ErrWithMsg(err, "Failed to search friends")
		}
	} else {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Message: "Already friends",
		}))
	}
	now := time.Now().Round(0).Unix()
	if err := s.invitations.DeleteByID(userid, invitedBy); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove friend invitation")
	}
	if err := s.invitations.Insert(userid, invitedBy, now); err != nil {
		return governor.ErrWithMsg(err, "Failed to add friend invitation")
	}
	return nil
}

func (s *service) AcceptFriendInvitation(userid, inviter string) error {
	m, err := s.users.GetByID(userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}
	m2, err := s.users.GetByID(inviter)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "User not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime
	if _, err := s.invitations.GetByID(userid, inviter, after); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "Friend invitation not found",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get friend invitation")
	}

	b, err := json.Marshal(FriendProps{
		Userid:    m.Userid,
		InvitedBy: m2.Userid,
	})
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to encode friend props to json")
	}

	if err := s.invitations.DeleteByID(userid, inviter); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete friend invitation")
	}
	if err := s.friends.Insert(m.Userid, m2.Userid, m.Username, m2.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to add friend")
	}
	if err := s.events.StreamPublish(s.opts.FriendChannel, b); err != nil {
		s.logger.Error("Failed to publish friend event", map[string]string{
			"error":      err.Error(),
			"actiontype": "publishfriend",
		})
	}
	return nil
}

func (s *service) DeleteFriendInvitation(userid, inviter string) error {
	if err := s.invitations.DeleteByID(userid, inviter); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete friend invitation")
	}
	return nil
}

type (
	resFriendInvitation struct {
		Userid       string `json:"userid"`
		InvitedBy    string `json:"invited_by"`
		CreationTime int64  `json:"creation_time"`
	}

	resFriendInvitations struct {
		Invitations []resFriendInvitation `json:"invitations"`
	}
)

func (s *service) GetUserFriendInvitations(userid string, amount, offset int) (*resFriendInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	m, err := s.invitations.GetByUser(userid, after, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get friend invitations")
	}
	res := make([]resFriendInvitation, 0, len(m))
	for _, i := range m {
		res = append(res, resFriendInvitation{
			Userid:       i.Userid,
			InvitedBy:    i.InvitedBy,
			CreationTime: i.CreationTime,
		})
	}
	return &resFriendInvitations{
		Invitations: res,
	}, nil
}

func (s *service) GetInvitedFriendInvitations(userid string, amount, offset int) (*resFriendInvitations, error) {
	now := time.Now().Round(0).Unix()
	after := now - s.invitationTime

	m, err := s.invitations.GetByInviter(userid, after, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get friend invitations")
	}
	res := make([]resFriendInvitation, 0, len(m))
	for _, i := range m {
		res = append(res, resFriendInvitation{
			Userid:       i.Userid,
			InvitedBy:    i.InvitedBy,
			CreationTime: i.CreationTime,
		})
	}
	return &resFriendInvitations{
		Invitations: res,
	}, nil
}

func (s *service) friendSubscriber(pinger events.Pinger, msgdata []byte) error {
	msg, err := DecodeFriendProps(msgdata)
	if err != nil {
		return err
	}
	m, err := s.dms.New(msg.InvitedBy, msg.Userid)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to create new dm")
	}
	if err := s.dms.Insert(m); err != nil {
		if !errors.Is(err, db.ErrUnique{}) {
			return governor.ErrWithMsg(err, "Failed to insert new dm")
		}
	}
	return nil
}

func (s *service) unfriendSubscriber(pinger events.Pinger, msgdata []byte) error {
	msg, err := DecodeUnfriendProps(msgdata)
	if err != nil {
		return err
	}
	m, err := s.dms.GetByID(msg.Userid, msg.Other)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get dm")
	}
	if err := s.msgs.DeleteChatMsgs(m.Chatid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete dm msgs")
	}
	if err := s.dms.Delete(msg.Userid, msg.Other); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete dm")
	}
	return nil
}
