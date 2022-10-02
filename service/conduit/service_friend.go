package conduit

import (
	"context"
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	resFriends struct {
		Friends []string `json:"friends"`
	}
)

func (s *Service) getFriends(ctx context.Context, userid string, prefix string, limit, offset int) (*resFriends, error) {
	m, err := s.friends.GetFriends(ctx, userid, prefix, limit, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to search friends")
	}
	res := make([]string, 0, len(m))
	for _, i := range m {
		res = append(res, i.Userid2)
	}
	return &resFriends{
		Friends: res,
	}, nil
}

type (
	resFriendSearch struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
	}

	resFriendSearches struct {
		Friends []resFriendSearch `json:"friends"`
	}
)

func (s *Service) searchFriends(ctx context.Context, userid string, prefix string, limit int) (*resFriendSearches, error) {
	m, err := s.friends.GetFriends(ctx, userid, prefix, limit, 0)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to search friends")
	}
	res := make([]resFriendSearch, 0, len(m))
	for _, i := range m {
		res = append(res, resFriendSearch{
			Userid:   i.Userid2,
			Username: i.Username,
		})
	}
	return &resFriendSearches{
		Friends: res,
	}, nil
}

func tupleSort(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}

func (s *Service) removeFriend(ctx context.Context, userid1, userid2 string) error {
	m, err := s.friends.GetByID(ctx, userid1, userid2)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Friend not found")
		}
		return kerrors.WithMsg(err, "Failed to get friends")
	}
	b, err := encodeConduitEventUnfriend(unfriendProps{
		Userid: userid1,
		Other:  userid2,
	})
	if err != nil {
		return err
	}
	u1, u2 := tupleSort(m.Userid1, m.Userid2)
	if err := s.events.Publish(ctx, events.NewMsgs(s.streamconduit, u1+"."+u2, b)...); err != nil {
		return kerrors.WithMsg(err, "Failed to publish unfriend event")
	}
	if err := s.friends.Remove(ctx, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to remove friend")
	}
	return nil
}

func (s *Service) inviteFriend(ctx context.Context, userid string, invitedBy string) error {
	if _, err := s.friends.GetByID(ctx, userid, invitedBy); err != nil {
		if !errors.Is(err, db.ErrorNotFound{}) {
			return kerrors.WithMsg(err, "Failed to search friends")
		}
	} else {
		return governor.ErrWithRes(err, http.StatusBadRequest, "", "Already friends")
	}
	now := time.Now().Round(0).Unix()
	if err := s.invitations.DeleteByID(ctx, userid, invitedBy); err != nil {
		return kerrors.WithMsg(err, "Failed to remove friend invitation")
	}
	if err := s.invitations.Insert(ctx, userid, invitedBy, now); err != nil {
		return kerrors.WithMsg(err, "Failed to add friend invitation")
	}
	return nil
}

func (s *Service) acceptFriendInvitation(ctx context.Context, userid, inviter string) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	m2, err := s.users.GetByID(ctx, inviter)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}

	after := time.Now().Round(0).Add(-s.invitationDuration).Unix()
	if _, err := s.invitations.GetByID(ctx, userid, inviter, after); err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "Friend invitation not found")
		}
		return kerrors.WithMsg(err, "Failed to get friend invitation")
	}

	b, err := encodeConduitEventFriend(friendProps{
		Userid:    m.Userid,
		InvitedBy: m2.Userid,
	})
	if err != nil {
		return err
	}

	if err := s.invitations.DeleteByID(ctx, userid, inviter); err != nil {
		return kerrors.WithMsg(err, "Failed to delete friend invitation")
	}
	if err := s.friends.Insert(ctx, m.Userid, m2.Userid, m.Username, m2.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to add friend")
	}

	// must make best effort attempt to publish friend event
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)
	u1, u2 := tupleSort(m.Userid, m2.Userid)
	if err := s.events.Publish(ctx, events.NewMsgs(s.streamconduit, u1+"."+u2, b)...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish friend event"), nil)
	}
	return nil
}

func (s *Service) deleteFriendInvitation(ctx context.Context, userid, inviter string) error {
	if err := s.invitations.DeleteByID(ctx, userid, inviter); err != nil {
		return kerrors.WithMsg(err, "Failed to delete friend invitation")
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

func (s *Service) getUserFriendInvitations(ctx context.Context, userid string, amount, offset int) (*resFriendInvitations, error) {
	after := time.Now().Round(0).Add(-s.invitationDuration).Unix()

	m, err := s.invitations.GetByUser(ctx, userid, after, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get friend invitations")
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

func (s *Service) getInvitedFriendInvitations(ctx context.Context, userid string, amount, offset int) (*resFriendInvitations, error) {
	after := time.Now().Round(0).Add(-s.invitationDuration).Unix()

	m, err := s.invitations.GetByInviter(ctx, userid, after, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get friend invitations")
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

func (s *Service) friendEventHandler(ctx context.Context, props friendProps) error {
	m, err := s.dms.New(props.InvitedBy, props.Userid)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to create new dm")
	}
	if err := s.dms.Insert(ctx, m); err != nil {
		if !errors.Is(err, db.ErrorUnique{}) {
			return kerrors.WithMsg(err, "Failed to insert new dm")
		}
	}
	return nil
}

func (s *Service) unfriendEventHandler(ctx context.Context, props unfriendProps) error {
	m, err := s.dms.GetByID(ctx, props.Userid, props.Other)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound{}) {
			// TODO: emit dm delete event
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get dm")
	}
	if err := s.msgs.DeleteChatMsgs(ctx, m.Chatid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete dm msgs")
	}
	if err := s.dms.Delete(ctx, props.Userid, props.Other); err != nil {
		return kerrors.WithMsg(err, "Failed to delete dm")
	}
	// TODO: emit dm delete event
	return nil
}

func (s *Service) rmFriend(ctx context.Context, userid1, userid2 string) error {
	if m, err := s.dms.GetByID(ctx, userid1, userid2); err != nil {
		if !errors.Is(err, db.ErrorNotFound{}) {
			return kerrors.WithMsg(err, "Failed to get dm")
		}
	} else {
		if err := s.msgs.DeleteChatMsgs(ctx, m.Chatid); err != nil {
			return kerrors.WithMsg(err, "Failed to delete dm msgs")
		}
		if err := s.dms.Delete(ctx, userid1, userid2); err != nil {
			return kerrors.WithMsg(err, "Failed to delete dm")
		}
	}
	// TODO: emit dm delete event
	if err := s.friends.Remove(ctx, userid1, userid2); err != nil {
		return kerrors.WithMsg(err, "Failed to remove friend")
	}
	return nil
}
