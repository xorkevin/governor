package authzacl

import (
	"context"

	"xorkevin.dev/governor/service/authzacl/aclmodel"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	aclEvent struct {
		Add bool     `json:"add"`
		Rel Relation `json:"rel"`
	}
)

func (s *Service) InsertRelations(ctx context.Context, rels []Relation) error {
	if len(rels) == 0 {
		return nil
	}

	m := make([]*aclmodel.Model, 0, len(rels))
	msgs := make([]events.PublishMsg, 0, len(rels))
	for _, i := range rels {
		m = append(m, &aclmodel.Model{
			ObjNS:   i.Obj.NS,
			ObjKey:  i.Obj.Key,
			ObjPred: i.Obj.Pred,
			SubNS:   i.Sub.NS,
			SubKey:  i.Sub.Key,
			SubPred: i.Sub.Pred,
		})
		b, err := kjson.Marshal(aclEvent{
			Add: true,
			Rel: i,
		})
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encode add relation acl event")
		}
		msgkey := i.Sub.NS + ":" + i.Sub.Key
		if i.Sub.Pred != "" {
			msgkey += "#" + i.Sub.Pred
		}
		msgs = append(msgs, events.PublishMsg{
			Topic: s.streamacl,
			Key:   msgkey,
			Value: b,
		})
	}

	if err := s.repo.Insert(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to add relations")
	}

	// must make a best effort to publish tuple event
	ctx = klog.ExtendCtx(context.Background(), ctx)
	if err := s.events.Publish(ctx, msgs...); err != nil {
		return kerrors.WithMsg(err, "Failed to publish add relations acl event")
	}
	return nil
}

func (s *Service) DeleteRelations(ctx context.Context, rels []Relation) error {
	if len(rels) == 0 {
		return nil
	}

	m := make([]aclmodel.Model, 0, len(rels))
	msgs := make([]events.PublishMsg, 0, len(rels))
	for _, i := range rels {
		m = append(m, aclmodel.Model{
			ObjNS:   i.Obj.NS,
			ObjKey:  i.Obj.Key,
			ObjPred: i.Obj.Pred,
			SubNS:   i.Sub.NS,
			SubKey:  i.Sub.Key,
			SubPred: i.Sub.Pred,
		})
		b, err := kjson.Marshal(aclEvent{
			Add: false,
			Rel: i,
		})
		if err != nil {
			return kerrors.WithMsg(err, "Failed to encode remove relation acl event")
		}
		msgkey := i.Sub.NS + ":" + i.Sub.Key
		if i.Sub.Pred != "" {
			msgkey += "#" + i.Sub.Pred
		}
		msgs = append(msgs, events.PublishMsg{
			Topic: s.streamacl,
			Key:   msgkey,
			Value: b,
		})
	}

	if err := s.events.Publish(ctx, msgs...); err != nil {
		return kerrors.WithMsg(err, "Failed to publish remove relations acl event")
	}

	if err := s.repo.Delete(ctx, m); err != nil {
		return kerrors.WithMsg(err, "Failed to remove relations")
	}
	return nil
}

func (s *Service) Read(ctx context.Context, obj ObjRel, after *Sub, limit int) ([]Sub, error) {
	var cursor aclmodel.Subject
	if after != nil {
		cursor = aclmodel.Subject{
			SubNS:   after.NS,
			SubKey:  after.Key,
			SubPred: after.Pred,
		}
	}
	m, err := s.repo.Read(ctx, aclmodel.ObjectRel{
		ObjNS:   obj.NS,
		ObjKey:  obj.Key,
		ObjPred: obj.Pred,
	}, cursor, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get object relations")
	}
	res := make([]Sub, 0, len(m))
	for _, i := range m {
		res = append(res, Sub{
			NS:   i.SubNS,
			Key:  i.SubKey,
			Pred: i.SubPred,
		})
	}
	return res, nil
}

func (s *Service) ReadBySubObjPred(ctx context.Context, sub Sub, objns, pred, afterKey string, limit int) ([]string, error) {
	m, err := s.repo.ReadBySubObjPred(ctx, aclmodel.Subject{
		SubNS:   sub.NS,
		SubKey:  sub.Key,
		SubPred: sub.Pred,
	}, objns, pred, afterKey, limit)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get subject relations")
	}
	return m, nil
}

func (s *Service) Check(ctx context.Context, obj Obj, pred string, sub Sub) (bool, error) {
	ok, err := s.repo.Check(
		ctx,
		aclmodel.Object{
			ObjNS:  obj.NS,
			ObjKey: obj.Key,
		},
		pred,
		aclmodel.Subject{
			SubNS:   sub.NS,
			SubKey:  sub.Key,
			SubPred: sub.Pred,
		},
	)
	if err != nil {
		return false, kerrors.WithMsg(err, "Failed to check acl relation")
	}
	return ok, nil
}
