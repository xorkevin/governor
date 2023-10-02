package apikey

import (
	"context"
	"crypto/hmac"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

var _ Apikeys = (*KeySet)(nil)

type (
	MemKey struct {
		Hash         []byte
		UserScope    UserScope
		Name         string
		Desc         string
		RotateTime   int64
		UpdateTime   int64
		CreationTime int64
	}

	KeySet struct {
		mu  sync.RWMutex
		Set map[string]MemKey
	}
)

func (s *KeySet) getKey(keyid string) (*MemKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k, ok := s.Set[keyid]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	return &k, nil
}

func (s *KeySet) GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]Props, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var keyids []string
	for k, v := range s.Set {
		if v.UserScope.Userid == userid {
			keyids = append(keyids, k)
		}
	}
	if offset >= len(keyids) {
		return nil, nil
	}
	slices.Sort(keyids)
	slices.Reverse(keyids)
	keyids = keyids[offset:min(offset+limit, len(keyids))]

	res := make([]Props, 0, len(keyids))
	for _, i := range keyids {
		k := s.Set[i]
		res = append(res, Props{
			Keyid:        i,
			Userid:       k.UserScope.Userid,
			Scope:        k.UserScope.Scope,
			Name:         k.Name,
			Desc:         k.Desc,
			RotateTime:   k.RotateTime,
			UpdateTime:   k.UpdateTime,
			CreationTime: k.CreationTime,
		})
	}
	return res, nil
}

func (s *KeySet) Check(ctx context.Context, keyid, key string) (*UserScope, error) {
	k, err := s.getKey(keyid)
	if err != nil {
		return nil, err
	}
	h := blake2b.Sum512([]byte(key))
	if !hmac.Equal(k.Hash, h[:]) {
		return nil, kerrors.WithKind(nil, ErrInvalidKey, "Invalid key")
	}
	m := k.UserScope
	return &m, nil
}

func (s *KeySet) InsertKey(ctx context.Context, userid string, scope string, name, desc string) (*Key, error) {
	u, err := uid.New()
	if err != nil {
		return nil, err
	}
	keyid := u.Base64()
	k, err := uid.NewKey()
	if err != nil {
		return nil, err
	}
	key := k.Base64()
	h := blake2b.Sum512([]byte(key))

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Round(0).Unix()

	s.Set[keyid] = MemKey{
		Hash: h[:],
		UserScope: UserScope{
			Userid: userid,
			Scope:  scope,
		},
		Name:         name,
		Desc:         desc,
		RotateTime:   now,
		UpdateTime:   now,
		CreationTime: now,
	}
	return &Key{
		Keyid: keyid,
		Key:   fmt.Sprintf("ga.%s.%s", keyid, key),
	}, nil
}

func (s *KeySet) RotateKey(ctx context.Context, userid string, keyid string) (*Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k, ok := s.Set[keyid]
	if !ok {
		return nil, kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	if userid != k.UserScope.Userid {
		return nil, kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}

	keybytes, err := uid.NewKey()
	if err != nil {
		return nil, err
	}
	key := keybytes.Base64()
	h := blake2b.Sum512([]byte(key))

	k.Hash = h[:]
	s.Set[keyid] = k

	return &Key{
		Keyid: keyid,
		Key:   fmt.Sprintf("ga.%s.%s", keyid, key),
	}, nil
}

func (s *KeySet) UpdateKey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k, ok := s.Set[keyid]
	if !ok {
		return kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	if userid != k.UserScope.Userid {
		return kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}

	k.UserScope.Scope = scope
	k.Name = name
	k.Desc = desc
	s.Set[keyid] = k

	return nil
}

func (s *KeySet) DeleteKey(ctx context.Context, userid string, keyid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k, ok := s.Set[keyid]
	if !ok {
		return kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	if userid != k.UserScope.Userid {
		return kerrors.WithKind(nil, ErrNotFound, "Key not found")
	}
	delete(s.Set, keyid)
	return nil
}

func (s *KeySet) DeleteUserKeys(ctx context.Context, userid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var keyids []string
	for k, v := range s.Set {
		if v.UserScope.Userid == userid {
			keyids = append(keyids, k)
		}
	}
	for _, i := range keyids {
		delete(s.Set, i)
	}
	return nil
}
