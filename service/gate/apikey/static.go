package apikey

import (
	"context"
	"crypto/hmac"
	"sync"

	"golang.org/x/crypto/blake2b"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
)

type (
	MemKey struct {
		Hash      []byte
		UserScope UserScope
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

func (s *KeySet) InsertKey(ctx context.Context, userid string, scope string, name, desc string) (*ResApikeyModel, error) {
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

	s.Set[keyid] = MemKey{
		Hash: h[:],
		UserScope: UserScope{
			Userid: userid,
			Scope:  scope,
		},
	}
	return &ResApikeyModel{
		Keyid: keyid,
		Key:   key,
	}, nil
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
