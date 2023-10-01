package user

import (
	"context"
	"errors"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

func (s *Service) updateUser(ctx context.Context, userid string, ruser reqUserPut) error {
	m, err := s.users.GetByID(ctx, userid)
	if err != nil {
		if errors.Is(err, dbsql.ErrNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
		}
		return kerrors.WithMsg(err, "Failed to get user")
	}
	updUsername := m.Username != ruser.Username
	m.Username = ruser.Username
	m.FirstName = ruser.FirstName
	m.LastName = ruser.LastName
	var b []byte
	if updUsername {
		var err error
		b, err = encodeUserEventUpdate(UpdateUserProps{
			Userid:   m.Userid,
			Username: m.Username,
		})
		if err != nil {
			return err
		}
	}

	if err = s.users.UpdateProps(ctx, m); err != nil {
		if errors.Is(err, dbsql.ErrUnique) {
			return governor.ErrWithRes(err, http.StatusBadRequest, "", "Username must be unique")
		}
		return kerrors.WithMsg(err, "Failed to update user")
	}

	if updUsername {
		// must make a best effort to publish username update
		ctx = klog.ExtendCtx(context.Background(), ctx)
		if err := s.events.Publish(ctx, events.NewMsgs(s.eventSettings.streamUsers, userid, b)...); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish update user props event"))
		}
	}
	return nil
}

func (s *Service) checkAdminOrMod(ctx context.Context, userid string, role string) error {
	if ok, err := gate.CheckRole(ctx, s.acl, userid, gate.RoleAdmin); err != nil {
		return kerrors.WithMsg(err, "Failed to get updater role")
	} else if ok {
		return nil
	}
	if ok, err := gate.CheckMod(ctx, s.acl, userid, role); err != nil {
		return kerrors.WithMsg(err, "Failed to get updater role")
	} else if ok {
		return nil
	}
	return governor.ErrWithRes(nil, http.StatusForbidden, "", "Not allowed to update role")
}

func (s *Service) updateRole(ctx context.Context, userid string, updaterid string, role string, mod bool, add bool) error {
	if role == gate.RoleAdmin && mod {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Invalid mod role")
	}

	if err := s.checkAdminOrMod(ctx, updaterid, role); err != nil {
		return err
	}

	if ok, err := s.users.Exists(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to get user")
	} else if !ok {
		return governor.ErrWithRes(err, http.StatusNotFound, "", "User not found")
	}

	if mod {
		if ok, err := gate.CheckMod(ctx, s.acl, userid, role); err != nil {
			return kerrors.WithMsg(err, "Failed to get user role")
		} else if !ok {
			return nil
		}
	} else {
		if ok, err := gate.CheckRole(ctx, s.acl, userid, role); err != nil {
			return kerrors.WithMsg(err, "Failed to get user role")
		} else if !ok {
			return nil
		}
	}

	pred := gate.RelIn
	if mod {
		pred = gate.RelMod
	}
	rel := authzacl.Rel(gate.NSRole, role, pred, gate.NSUser, userid, "")
	if add {
		if err := s.acl.InsertRelations(ctx, []authzacl.Relation{rel}); err != nil {
			return kerrors.WithMsg(err, "Failed to add role")
		}
	} else {
		if err := s.acl.DeleteRelations(ctx, []authzacl.Relation{rel}); err != nil {
			return kerrors.WithMsg(err, "Failed to remove role")
		}
	}
	return nil
}
