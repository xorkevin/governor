package authzacl

import (
	"context"
	"sync"
)

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

func (s *ACLSet) AddRelations(ctx context.Context, relations ...Relation) {
	s.InsertRelations(ctx, relations)
}

func (s *ACLSet) RmRelations(ctx context.Context, relations ...Relation) {
	s.DeleteRelations(ctx, relations)
}

func Rel(objns, objkey, objpred, subns, subkey, subpred string) Relation {
	return Relation{
		Obj: ObjRel{
			NS:   objns,
			Key:  objkey,
			Pred: objpred,
		},
		Sub: Sub{
			NS:   subns,
			Key:  subkey,
			Pred: subpred,
		},
	}
}
