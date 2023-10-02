package authzacl

import (
	"context"
	"slices"
	"sync"
)

var _ Manager = (*ACLSet)(nil)

type (
	ACLSet struct {
		mu  sync.RWMutex
		Set map[Relation]struct{}
	}
)

func (s *ACLSet) Check(ctx context.Context, obj Obj, pred string, sub Sub) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.Set[Relation{
		Obj: ObjRel{
			NS:   obj.NS,
			Key:  obj.Key,
			Pred: pred,
		},
		Sub: sub,
	}]
	return ok, nil
}

func (s *ACLSet) InsertRelations(ctx context.Context, relations []Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, i := range relations {
		s.Set[i] = struct{}{}
	}
	return nil
}

func (s *ACLSet) DeleteRelations(ctx context.Context, relations []Relation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, i := range relations {
		delete(s.Set, i)
	}
	return nil
}

func (s *ACLSet) Read(ctx context.Context, obj ObjRel, after *Sub, limit int) ([]Sub, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []Sub
	for k := range s.Set {
		if k.Obj == obj && (after == nil ||
			k.Sub.NS > after.NS ||
			k.Sub.NS == after.NS && k.Obj.Key > after.Key ||
			k.Sub.NS == after.NS && k.Obj.Key == after.Key && k.Sub.Pred > after.Pred) {
			res = append(res, k.Sub)
		}
	}
	slices.SortFunc(res, func(a, b Sub) int {
		if a.NS < b.NS {
			return -1
		}
		if a.NS > b.NS {
			return 1
		}
		if a.Key < b.Key {
			return -1
		}
		if a.Key > b.Key {
			return 1
		}
		if a.Pred < b.Pred {
			return -1
		}
		if a.Pred > b.Pred {
			return 1
		}
		return 0
	})
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (s *ACLSet) ReadBySubObjPred(ctx context.Context, sub Sub, objns, pred, afterKey string, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []string
	for k := range s.Set {
		if k.Sub == sub && k.Obj.NS == objns && k.Obj.Pred == pred && k.Obj.Key > afterKey {
			res = append(res, k.Obj.Key)
		}
	}
	slices.Sort(res)
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (s *ACLSet) AddRelations(ctx context.Context, relations ...Relation) {
	s.InsertRelations(ctx, relations)
}

func (s *ACLSet) RmRelations(ctx context.Context, relations ...Relation) {
	s.DeleteRelations(ctx, relations)
}
